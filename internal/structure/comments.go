package structure

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

const (
	commentPageSize = 50
	maxCommentBytes = 4096
	maxMentions     = 50
)

var mentionPattern = regexp.MustCompile(`@[A-Za-z0-9._-]+`)

type MentionUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type Comment struct {
	ID         string        `json:"id"`
	AuthorID   *string       `json:"authorId"`
	AuthorName string        `json:"authorName"`
	Body       string        `json:"body"`
	Mentions   []MentionUser `json:"mentions"`
	CreatedAt  time.Time     `json:"createdAt"`
	EditedAt   *time.Time    `json:"editedAt"`
}

type CommentListing struct {
	Items   []Comment `json:"items"`
	HasMore bool      `json:"hasMore"`
}

func mentionNames(body string) ([]string, error) {
	if !utf8.ValidString(body) || len(body) > maxCommentBytes || strings.TrimSpace(body) == "" ||
		strings.ContainsFunc(body, func(r rune) bool {
			return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
		}) {
		return nil, validation("Comment body is invalid or too long")
	}
	names := make([]string, 0)
	seen := make(map[string]bool)
	for _, location := range mentionPattern.FindAllStringIndex(body, -1) {
		if location[0] > 0 {
			previous := body[location[0]-1]
			if previous >= 'a' && previous <= 'z' || previous >= 'A' && previous <= 'Z' ||
				previous >= '0' && previous <= '9' || strings.ContainsRune("._-@", rune(previous)) {
				continue
			}
		}
		name := strings.ToLower(body[location[0]+1 : location[1]])
		if len(name) < 3 || len(name) > 32 || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if len(names) > maxMentions {
			return nil, validation("Comment has too many mentions")
		}
	}
	return names, nil
}

func lockCommentRow(ctx context.Context, tx pgx.Tx, table rowTable, id int64) error {
	query, args, err := sq.Select(`"id"`).From(table.qualifiedName()).
		Where(sq.Eq{`"id"`: id}).Suffix("FOR KEY SHARE").
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build comment row lookup: %w", err)
	}
	if err := tx.QueryRow(ctx, query, args...).Scan(new(int64)); errors.Is(err, pgx.ErrNoRows) {
		return acl.ErrNotFound()
	} else if err != nil {
		return fmt.Errorf("lock comment row: %w", err)
	}
	return nil
}

func (s *Service) eligibleMentions(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID, names []string) ([]pgtype.UUID, error) {
	result := make([]pgtype.UUID, 0, len(names))
	if len(names) == 0 {
		return result, nil
	}
	users, err := structuresql.New(tx).LockMentionUsers(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("lock mentioned users: %w", err)
	}
	target := acl.Resource{Type: acl.Table, ID: tableID}
	for _, user := range users {
		access, err := acl.AccessUncached(ctx, tx, user.ID, target)
		if err != nil {
			return nil, err
		}
		if access.Level >= acl.Read {
			result = append(result, user.ID)
		}
	}
	return result, nil
}

func mentionLabels(ctx context.Context, q *structuresql.Queries, ids []pgtype.UUID) (map[pgtype.UUID]MentionUser, error) {
	result := make(map[pgtype.UUID]MentionUser, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	users, err := q.GetMentionUsers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve comment mentions: %w", err)
	}
	for _, user := range users {
		result[user.ID] = MentionUser{ID: user.ID.String(), Username: user.Username, Name: user.Name}
	}
	return result, nil
}

func commentDTO(id, userID pgtype.UUID, author, body string, mentions []pgtype.UUID,
	created, edited pgtype.Timestamptz, labels map[pgtype.UUID]MentionUser) Comment {
	comment := Comment{
		ID: id.String(), AuthorName: author, Body: body, Mentions: make([]MentionUser, 0, len(mentions)),
		CreatedAt: created.Time,
	}
	if userID.Valid {
		value := userID.String()
		comment.AuthorID = &value
	}
	if edited.Valid {
		value := edited.Time
		comment.EditedAt = &value
	}
	for _, id := range mentions {
		if user, ok := labels[id]; ok {
			comment.Mentions = append(comment.Mentions, user)
		}
	}
	return comment
}

func loadComment(ctx context.Context, q *structuresql.Queries, id pgtype.UUID) (Comment, error) {
	row, err := q.FindRowComment(ctx, id)
	if err != nil {
		return Comment{}, notFoundIfNoRows(err, "find comment")
	}
	labels, err := mentionLabels(ctx, q, row.Mentions)
	if err != nil {
		return Comment{}, err
	}
	return commentDTO(row.ID, row.UserID, row.AuthorName, row.Body, row.Mentions, row.CreatedAt, row.EditedAt, labels), nil
}

func (s *Service) ListComments(ctx context.Context, actor Actor, tableID, rowID string, offset int, focus string) (CommentListing, error) {
	id, err := parsePathRowID(rowID)
	if err != nil {
		return CommentListing{}, err
	}
	if offset < 0 || offset > maxQueryOffset {
		return CommentListing{}, validation("Comment offset is outside the supported bounds")
	}
	var focusID pgtype.UUID
	if focus != "" {
		var ok bool
		focusID, ok = acl.ParseUUID(focus)
		if !ok {
			return CommentListing{}, validation("Comment focus is invalid")
		}
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return CommentListing{}, acl.ErrValidation()
	}
	result := CommentListing{Items: []Comment{}}
	err = s.withReadableTable(ctx, actor, tableID, func(tx pgx.Tx, table rowTable) error {
		if err := s.authorizeRows(ctx, tx, user, acl.Resource{Type: acl.Table, ID: table.id}, acl.Read); err != nil {
			return err
		}
		if err := lockCommentRow(ctx, tx, table, id); err != nil {
			return err
		}
		q := structuresql.New(tx)
		rows, err := q.ListRowComments(ctx, structuresql.ListRowCommentsParams{
			TableID: table.id, RowID: id, FocusID: focusID, PageOffset: int32(offset), PageLimit: commentPageSize + 1,
		})
		if err != nil {
			return fmt.Errorf("list row comments: %w", err)
		}
		if focusID.Valid && offset == 0 && (len(rows) == 0 || rows[0].ID != focusID) {
			return apperr.New(404, "ERR_COMMENT_NOT_FOUND", "Focused comment not found")
		}
		result.HasMore = len(rows) > commentPageSize
		if result.HasMore {
			rows = rows[:commentPageSize]
		}
		ids := make([]pgtype.UUID, 0)
		for _, row := range rows {
			ids = append(ids, row.Mentions...)
		}
		labels, err := mentionLabels(ctx, q, ids)
		if err != nil {
			return err
		}
		for _, row := range rows {
			result.Items = append(result.Items, commentDTO(
				row.ID, row.UserID, row.AuthorName, row.Body, row.Mentions, row.CreatedAt, row.EditedAt, labels))
		}
		return nil
	})
	return result, err
}

func uuidDifference(left, right []pgtype.UUID) []pgtype.UUID {
	excluded := make(map[pgtype.UUID]bool, len(right))
	for _, id := range right {
		excluded[id] = true
	}
	result := make([]pgtype.UUID, 0, len(left))
	for _, id := range left {
		if !excluded[id] {
			result = append(result, id)
		}
	}
	return result
}

func publishComment(ctx context.Context, tx pgx.Tx, actor Actor, tableID pgtype.UUID, rowID int64, kind string) error {
	return events.Emit(ctx, tx, events.Event{
		Type: kind, TableID: tableID.String(), IDs: []string{strconv.FormatInt(rowID, 10)},
		By: actor.ID, ByName: actor.Name,
	})
}

func notifyMentions(ctx context.Context, tx pgx.Tx, actor Actor, tableID, commentID pgtype.UUID, recipients []pgtype.UUID) error {
	q := structuresql.New(tx)
	for _, recipient := range recipients {
		if recipient.String() == actor.ID {
			continue
		}
		if _, err := q.InsertMentionNotification(ctx, structuresql.InsertMentionNotificationParams{
			UserID: recipient, CommentID: commentID,
		}); err != nil {
			return fmt.Errorf("insert mention notification: %w", err)
		}
		if err := events.Emit(ctx, tx, events.Event{
			Type: "notification", TableID: tableID.String(), Recipient: recipient.String(), By: actor.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func removeMentionNotifications(ctx context.Context, tx pgx.Tx, commentID pgtype.UUID, recipients []pgtype.UUID) error {
	if len(recipients) == 0 {
		return nil
	}
	if err := structuresql.New(tx).DeleteMentionNotifications(ctx, structuresql.DeleteMentionNotificationsParams{
		UserIds: recipients, CommentID: commentID.String(),
	}); err != nil {
		return fmt.Errorf("remove mention notifications: %w", err)
	}
	for _, recipient := range recipients {
		if err := events.Emit(ctx, tx, events.Event{Type: "notification.changed", Recipient: recipient.String()}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateComment(ctx context.Context, actor Actor, tableID, rowID, body string) (Comment, error) {
	id, err := parsePathRowID(rowID)
	if err != nil {
		return Comment{}, err
	}
	names, err := mentionNames(body)
	if err != nil {
		return Comment{}, err
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return Comment{}, acl.ErrValidation()
	}
	var result Comment
	err = s.withTableMutation(ctx, actor, tableID, acl.Read, func(tx pgx.Tx, table rowTable) error {
		if err := lockCommentRow(ctx, tx, table, id); err != nil {
			return err
		}
		mentions, err := s.eligibleMentions(ctx, tx, table.id, names)
		if err != nil {
			return err
		}
		q := structuresql.New(tx)
		commentID, err := q.InsertRowComment(ctx, structuresql.InsertRowCommentParams{
			TableID: table.id, RowID: id, UserID: user, Body: body, Mentions: mentions,
		})
		if err != nil {
			return fmt.Errorf("insert comment: %w", err)
		}
		if err := notifyMentions(ctx, tx, actor, table.id, commentID, mentions); err != nil {
			return err
		}
		if err := publishComment(ctx, tx, actor, table.id, id, "comment.created"); err != nil {
			return err
		}
		result, err = loadComment(ctx, q, commentID)
		return err
	})
	return result, err
}

func (s *Service) changeComment(ctx context.Context, actor Actor, commentID string, body *string) (Comment, error) {
	id, ok := acl.ParseUUID(commentID)
	if !ok {
		return Comment{}, acl.ErrNotFound()
	}
	var names []string
	var err error
	if body != nil {
		names, err = mentionNames(*body)
		if err != nil {
			return Comment{}, err
		}
	}
	prior, err := s.queries.FindRowComment(ctx, id)
	if err != nil {
		return Comment{}, notFoundIfNoRows(err, "find comment")
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return Comment{}, acl.ErrValidation()
	}
	var result Comment
	err = s.withTableMutation(ctx, actor, prior.TableID.String(), acl.Read, func(tx pgx.Tx, table rowTable) error {
		if err := lockCommentRow(ctx, tx, table, prior.RowID); err != nil {
			return err
		}
		q := structuresql.New(tx)
		locked, err := q.LockRowComment(ctx, id)
		if err != nil {
			return notFoundIfNoRows(err, "lock comment")
		}
		if locked.TableID != table.id || locked.RowID != prior.RowID {
			return acl.ErrNotFound()
		}
		access, err := acl.AccessUncached(ctx, tx, user, acl.Resource{Type: acl.Table, ID: table.id})
		if err != nil {
			return err
		}
		if locked.UserID != user && (body != nil || access.Level < acl.Owner) {
			return apperr.New(403, "ERR_PERMISSION_DENIED", "Comment modification is not permitted")
		}
		if body == nil {
			if err := q.DeleteRowComment(ctx, id); err != nil {
				return fmt.Errorf("delete comment: %w", err)
			}
			if err := removeMentionNotifications(ctx, tx, id, locked.Mentions); err != nil {
				return err
			}
			return publishComment(ctx, tx, actor, table.id, locked.RowID, "comment.deleted")
		}
		mentions, err := s.eligibleMentions(ctx, tx, table.id, names)
		if err != nil {
			return err
		}
		if err := q.UpdateRowComment(ctx, structuresql.UpdateRowCommentParams{
			ID: id, Body: *body, Mentions: mentions,
		}); err != nil {
			return fmt.Errorf("update comment: %w", err)
		}
		if err := removeMentionNotifications(ctx, tx, id, uuidDifference(locked.Mentions, mentions)); err != nil {
			return err
		}
		if err := notifyMentions(ctx, tx, actor, table.id, id, uuidDifference(mentions, locked.Mentions)); err != nil {
			return err
		}
		if err := publishComment(ctx, tx, actor, table.id, locked.RowID, "comment.updated"); err != nil {
			return err
		}
		result, err = loadComment(ctx, q, id)
		return err
	})
	return result, err
}

func (s *Service) UpdateComment(ctx context.Context, actor Actor, id, body string) (Comment, error) {
	return s.changeComment(ctx, actor, id, &body)
}

func (s *Service) DeleteComment(ctx context.Context, actor Actor, id string) error {
	_, err := s.changeComment(ctx, actor, id, nil)
	return err
}

func enrichCommentCounts(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID, records []RowRecord) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		id, err := strconv.ParseInt(record.ID, 10, 64)
		if err != nil {
			return fmt.Errorf("parse row ID for comment count: %w", err)
		}
		ids = append(ids, id)
	}
	counts, err := structuresql.New(tx).CountRowComments(ctx, structuresql.CountRowCommentsParams{
		TableID: tableID, RowIds: ids,
	})
	if err != nil {
		return fmt.Errorf("count row comments: %w", err)
	}
	byID := make(map[int64]int64, len(counts))
	for _, count := range counts {
		byID[count.RowID] = count.CommentCount
	}
	for index, id := range ids {
		records[index].CommentCount = byID[id]
	}
	return nil
}

func enrichSingleCommentCount(ctx context.Context, tx pgx.Tx, tableID pgtype.UUID, record *RowRecord) error {
	records := []RowRecord{*record}
	if err := enrichCommentCounts(ctx, tx, tableID, records); err != nil {
		return err
	}
	record.CommentCount = records[0].CommentCount
	return nil
}
