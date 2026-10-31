package structure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

type MentionNotification struct {
	ID         string     `json:"id"`
	TableID    string     `json:"tableId"`
	TableName  string     `json:"tableName"`
	RowID      string     `json:"rowId"`
	CommentID  string     `json:"commentId"`
	AuthorName string     `json:"authorName"`
	CreatedAt  time.Time  `json:"createdAt"`
	ReadAt     *time.Time `json:"readAt"`
}

type NotificationListing struct {
	Items       []MentionNotification `json:"items"`
	UnreadCount int64                 `json:"unreadCount"`
	HasMore     bool                  `json:"hasMore"`
}

func (s *Service) ListNotifications(ctx context.Context, actor Actor, offset int) (NotificationListing, error) {
	if offset < 0 || offset > maxQueryOffset {
		return NotificationListing{}, validation("Notification offset is outside the supported bounds")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return NotificationListing{}, acl.ErrValidation()
	}
	row, err := s.queries.ListMentionNotifications(ctx, structuresql.ListMentionNotificationsParams{
		UserID: user, PageOffset: int32(offset), PageLimit: commentPageSize + 1,
	})
	if err != nil {
		return NotificationListing{}, fmt.Errorf("list mention notifications: %w", err)
	}
	result := NotificationListing{Items: []MentionNotification{}, UnreadCount: row.UnreadCount}
	if err := json.Unmarshal([]byte(row.Items), &result.Items); err != nil {
		return NotificationListing{}, fmt.Errorf("decode mention notifications: %w", err)
	}
	result.HasMore = len(result.Items) > commentPageSize
	if result.HasMore {
		result.Items = result.Items[:commentPageSize]
	}
	return result, nil
}

func (s *Service) ReadNotification(ctx context.Context, actor Actor, notificationID string) error {
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	id, ok := acl.ParseUUID(notificationID)
	if !ok {
		return validation("Notification ID is invalid")
	}
	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set notification lock timeout: %w", err)
		}
		if err := acl.LockActiveUser(ctx, tx, user); err != nil {
			return err
		}
		changed, err := structuresql.New(tx).MarkMentionNotificationRead(ctx, structuresql.MarkMentionNotificationReadParams{
			ID: id, UserID: user,
		})
		if err != nil {
			return fmt.Errorf("mark notification read: %w", err)
		}
		if changed > 0 {
			return events.Emit(ctx, tx, events.Event{Type: "notification.changed", Recipient: actor.ID, By: actor.ID})
		}
		return nil
	})
	return rowDatabaseError(err)
}
