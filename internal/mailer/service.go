package mailer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/admin"
	"pandatable/internal/apperr"
	"pandatable/internal/auth"
	"pandatable/internal/db"
	"pandatable/internal/mailer/mailersql"
	"pandatable/internal/smtpmail"
)

type Service struct {
	pool   *db.Pool
	auth   *auth.Service
	admin  *admin.Service
	origin string
	logger *slog.Logger
	resets chan string
}

func New(pool *db.Pool, authentication *auth.Service, management *admin.Service, baseURL string, logger *slog.Logger) (*Service, error) {
	if pool == nil || pool.Pool == nil || authentication == nil || management == nil || logger == nil {
		return nil, errors.New("mailer requires database, auth, admin and logger")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("mailer requires a valid application URL")
	}
	return &Service{pool: pool, auth: authentication, admin: management,
		origin: parsed.Scheme + "://" + parsed.Host, logger: logger, resets: make(chan string, 64)}, nil
}

func (service *Service) QueueReset(email string) {
	select {
	case service.resets <- email:
	default:
		service.logger.Warn("password reset mail queue is full")
	}
}

func (service *Service) Test(ctx context.Context, recipient, locale string) error {
	config, err := service.admin.DeliverySMTP(ctx)
	if err != nil {
		return err
	}
	subject, body := "PandaTable test email", "SMTP is configured for PandaTable."
	if locale == "zh-CN" {
		subject, body = "PandaTable 测试邮件", "PandaTable 的 SMTP 设置已生效。"
	}
	if err := smtpmail.Send(ctx, config, recipient, subject, body); err != nil {
		service.logger.Warn("SMTP test delivery failed")
		return apperr.New(502, "ERR_SMTP_SEND", "SMTP delivery failed")
	}
	return nil
}

func (service *Service) reset(ctx context.Context, email string) {
	config, err := service.admin.DeliverySMTP(ctx)
	if err != nil || !config.Enabled {
		service.logger.Warn("password reset mail unavailable")
		return
	}
	message, found, err := service.auth.IssuePasswordReset(ctx, email)
	if err != nil {
		service.logger.Error("password reset preparation failed")
		return
	}
	if !found {
		return
	}
	link := service.origin + "/reset-password#token=" + url.QueryEscape(message.Token)
	subject := "Reset your PandaTable password"
	body := "Use this link within 30 minutes to reset your password:\n" + link + "\n"
	if message.Locale == "zh-CN" {
		subject = "重置 PandaTable 密码"
		body = "请在 30 分钟内使用此链接重置密码：\n" + link + "\n"
	}
	if err := smtpmail.Send(ctx, config, message.Email, subject, body); err != nil {
		service.logger.Warn("password reset mail delivery failed")
	}
}

func (service *Service) mention(ctx context.Context) (bool, error) {
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin mention mail: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := mailersql.New(tx)
	job, err := q.ClaimMentionMail(ctx, pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim mention mail: %w", err)
	}
	target, err := q.MentionMailTarget(ctx, job.NotificationID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = q.DeleteMentionMail(ctx, job.NotificationID)
	} else if err == nil {
		var access acl.Access
		access, err = acl.AccessUncached(ctx, tx, target.UserID, acl.Resource{Type: acl.Table, ID: target.TableID})
		if err == nil && (!access.Visible || access.Level < acl.Read) {
			err = q.DeleteMentionMail(ctx, job.NotificationID)
		} else if err == nil {
			err = service.sendMention(ctx, q, job, target)
		}
	}
	if err != nil {
		return true, fmt.Errorf("process mention mail: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return true, fmt.Errorf("commit mention mail: %w", err)
	}
	return true, nil
}

func (service *Service) sendMention(ctx context.Context, q *mailersql.Queries, job mailersql.ClaimMentionMailRow, target mailersql.MentionMailTargetRow) error {
	config, err := service.admin.DeliverySMTP(ctx)
	if err == nil {
		subject := "You were mentioned in PandaTable"
		body := "You have a new mention. Sign in to view your notifications:\n" + service.origin + "/home\n"
		if target.Locale == "zh-CN" {
			subject = "有人在 PandaTable 中提到了你"
			body = "你有一条新的提及。请登录查看通知：\n" + service.origin + "/home\n"
		}
		err = smtpmail.Send(ctx, config, target.Email.String, subject, body)
	}
	if err == nil {
		return q.DeleteMentionMail(ctx, job.NotificationID)
	}
	service.logger.Warn("mention email delivery failed", "attempt", job.Attempts+1)
	if job.Attempts >= 2 {
		return q.DeleteMentionMail(ctx, job.NotificationID)
	}
	delay := 15 * time.Second
	if job.Attempts == 1 {
		delay = time.Minute
	}
	return q.RetryMentionMail(ctx, mailersql.RetryMentionMailParams{
		NotificationID: job.NotificationID,
		NextAttemptAt:  pgtype.Timestamptz{Time: time.Now().Add(delay).UTC(), Valid: true},
	})
}

func (service *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case email := <-service.resets:
			work, cancel := context.WithTimeout(ctx, 10*time.Second)
			service.reset(work, email)
			cancel()
		case <-ticker.C:
			for range 10 {
				work, cancel := context.WithTimeout(ctx, 10*time.Second)
				found, err := service.mention(work)
				cancel()
				if err != nil {
					service.logger.Error("mention email processing failed")
					break
				}
				if !found {
					break
				}
			}
		}
	}
}
