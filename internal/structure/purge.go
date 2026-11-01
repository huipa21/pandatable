package structure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/ddl"
	"pandatable/internal/events"
	"pandatable/internal/structure/structuresql"
)

const trashRetentionInterval = 24 * time.Hour

// PurgeFileStore removes an attachment and its optional thumbnail idempotently.
type PurgeFileStore interface {
	RemovePurgedFile(ctx context.Context, id, storedPath string, hasThumb bool) error
}

type purgeCandidate struct {
	kind      string
	id        pgtype.UUID
	workspace pgtype.UUID
}

var errProtectedTrash = errors.New("a newer trashed descendant is still retained")

func (s *Service) trashCandidates(ctx context.Context, cutoff time.Time) ([]purgeCandidate, error) {
	rows, err := s.queries.ListExpiredTrash(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list expired trash: %w", err)
	}
	result := make([]purgeCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, purgeCandidate{kind: row.Kind, id: row.ID, workspace: row.WorkspaceID})
	}
	return result, nil
}

func candidateStillExpired(ctx context.Context, tx pgx.Tx, candidate purgeCandidate, cutoff time.Time) (bool, error) {
	switch candidate.kind {
	case "workspace", "book", "table", "field", "view":
	default:
		return false, errors.New("invalid purge candidate kind")
	}
	expired, err := structuresql.New(tx).TrashCandidateExpired(ctx, structuresql.TrashCandidateExpiredParams{
		Kind: candidate.kind, ID: candidate.id, Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("recheck expired trash: %w", err)
	}
	return expired, nil
}

func hasProtectedDescendants(ctx context.Context, tx pgx.Tx, candidate purgeCandidate, cutoff time.Time) (bool, error) {
	switch candidate.kind {
	case "workspace", "book", "table":
	case "field", "view":
		return false, nil
	default:
		return false, errors.New("invalid purge candidate kind")
	}
	protected, err := structuresql.New(tx).TrashHasRetainedDescendants(ctx, structuresql.TrashHasRetainedDescendantsParams{
		Kind: candidate.kind, ID: candidate.id, Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("check retained descendants: %w", err)
	}
	return protected, nil
}

func (s *Service) purgeCandidate(ctx context.Context, candidate purgeCandidate, now time.Time) (bool, error) {
	written := false
	err := s.pool.WithTx(ctx, func(tx pgx.Tx) error {
		queries := s.queries.WithTx(tx)
		if _, err := tx.Exec(ctx, lockTimeout); err != nil {
			return fmt.Errorf("set trash purge lock timeout: %w", err)
		}
		if _, err := queries.LockTrashWorkspace(ctx, candidate.workspace); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("lock trash workspace: %w", err)
		}
		cutoff, _, err := trashRetention(ctx, tx, now)
		if err != nil {
			return err
		}
		expired, err := candidateStillExpired(ctx, tx, candidate, cutoff)
		if err != nil || !expired {
			return err
		}
		protected, err := hasProtectedDescendants(ctx, tx, candidate, cutoff)
		if err != nil {
			return err
		}
		if protected {
			return nil
		}
		switch candidate.kind {
		case "workspace", "book", "table":
			err = purgeTree(ctx, tx, candidate, cutoff, pgtype.UUID{})
		case "field":
			err = purgeField(ctx, tx, candidate.id, cutoff)
		case "view":
			err = queries.DeletePurgedView(ctx, candidate.id)
		}
		if errors.Is(err, errProtectedTrash) {
			return nil
		}
		if err != nil {
			return err
		}
		written = true
		return events.Emit(ctx, tx, events.Event{Type: "nav.changed"})
	})
	if written {
		s.resolver.Invalidate()
	}
	return written && err == nil, sanitizeLock(err)
}

func purgeTree(ctx context.Context, tx pgx.Tx, candidate purgeCandidate, cutoff time.Time, cleanupUser pgtype.UUID) error {
	switch candidate.kind {
	case "workspace", "book", "table":
	default:
		return errors.New("invalid purge tree kind")
	}
	queries := structuresql.New(tx)
	books, err := queries.ListPurgeBooks(ctx, structuresql.ListPurgeBooksParams{
		Kind: candidate.kind, ParentID: candidate.id,
	})
	if err != nil {
		return fmt.Errorf("list purge schemas: %w", err)
	}
	tables, err := queries.ListPurgeTables(ctx, structuresql.ListPurgeTablesParams{
		Kind: candidate.kind, ParentID: candidate.id,
	})
	if err != nil {
		return fmt.Errorf("list purge tables: %w", err)
	}
	tableIDs := make([]pgtype.UUID, 0, len(tables))
	tableSet := make(map[pgtype.UUID]bool)
	for _, table := range tables {
		tableIDs = append(tableIDs, table.ID)
		tableSet[table.ID] = true
	}
	if len(tableIDs) > 0 {
		if err := purgeTableRelations(ctx, tx, tableIDs, tableSet, cutoff); err != nil {
			return err
		}
		if err := queueTableAttachments(ctx, tx, tableIDs, cleanupUser); err != nil {
			return err
		}
		if err := queries.DeletePurgedGrants(ctx, structuresql.DeletePurgedGrantsParams{
			ResourceType: "table", Ids: tableIDs,
		}); err != nil {
			return fmt.Errorf("delete purged table grants: %w", err)
		}
	}
	for _, table := range tables {
		if err := ddl.DropTable(ctx, tx, ddl.TableName{Schema: table.PgSchema, Name: table.PgName}); err != nil {
			return err
		}
	}
	bookIDs := make([]pgtype.UUID, 0, len(books))
	for _, book := range books {
		if err := ddl.DropSchema(ctx, tx, book.PgSchema); err != nil {
			return err
		}
		bookIDs = append(bookIDs, book.ID)
	}
	if len(bookIDs) > 0 {
		if err := queries.DeletePurgedGrants(ctx, structuresql.DeletePurgedGrantsParams{
			ResourceType: "book", Ids: bookIDs,
		}); err != nil {
			return fmt.Errorf("delete purged book grants: %w", err)
		}
	}
	switch candidate.kind {
	case "workspace":
		if err := queries.DeletePurgedGrants(ctx, structuresql.DeletePurgedGrantsParams{
			ResourceType: "workspace", Ids: []pgtype.UUID{candidate.id},
		}); err != nil {
			return fmt.Errorf("delete purged workspace grants: %w", err)
		}
		err = queries.DeletePurgedWorkspace(ctx, candidate.id)
	case "book":
		err = queries.DeletePurgedBook(ctx, candidate.id)
	case "table":
		err = queries.DeletePurgedTable(ctx, candidate.id)
	}
	if err != nil {
		return fmt.Errorf("delete purged metadata: %w", err)
	}
	return nil
}

func queueTableAttachments(ctx context.Context, tx pgx.Tx, tableIDs []pgtype.UUID, cleanupUser pgtype.UUID) error {
	if err := structuresql.New(tx).QueuePurgedTableAttachments(ctx, structuresql.QueuePurgedTableAttachmentsParams{
		Ids: tableIDs, CleanupUserID: cleanupUser,
	}); err != nil {
		return fmt.Errorf("queue purged table attachments: %w", err)
	}
	return nil
}

func queueFieldAttachments(ctx context.Context, tx pgx.Tx, fieldIDs []pgtype.UUID) error {
	if err := structuresql.New(tx).QueuePurgedFieldAttachments(ctx, fieldIDs); err != nil {
		return fmt.Errorf("queue purged field attachments: %w", err)
	}
	return nil
}

func purgeTableRelations(ctx context.Context, tx pgx.Tx, tableIDs []pgtype.UUID,
	tableSet map[pgtype.UUID]bool, cutoff time.Time) error {
	queries := structuresql.New(tx)
	rows, err := queries.ListPurgeRelations(ctx, tableIDs)
	if err != nil {
		return fmt.Errorf("list purge relations: %w", err)
	}
	relations := make([]relationRecord, 0, len(rows))
	for _, row := range rows {
		relations = append(relations, relationFromMetadata(row))
	}
	fieldRows, err := queries.ListPurgeFieldIDs(ctx, tableIDs)
	if err != nil {
		return fmt.Errorf("list purged field dependencies: %w", err)
	}
	removedIDs := make([]string, 0, len(fieldRows))
	for _, id := range fieldRows {
		removedIDs = append(removedIDs, id.String())
	}
	affected := make(map[pgtype.UUID]bool)
	for _, relation := range relations {
		for _, pair := range []struct {
			table, field pgtype.UUID
		}{{relation.parentTable, relation.parentField}, {relation.childTable, relation.childField}} {
			removedIDs = append(removedIDs, pair.field.String())
			if tableSet[pair.table] {
				continue
			}
			deleted, err := queries.LockPurgeFieldDeletionTime(ctx, pair.field)
			if err != nil {
				return fmt.Errorf("read surviving relation field: %w", err)
			}
			if deleted.Valid && !deleted.Time.Before(cutoff) {
				return errProtectedTrash
			}
			affected[pair.table] = true
		}
	}
	if err := markPurgedDependencies(ctx, tx, tableIDs, removedIDs, affected); err != nil {
		return err
	}
	for _, relation := range relations {
		if err := dropRelationStorage(ctx, tx, relation); err != nil {
			return err
		}
		if err := deletePurgedRelation(ctx, queries, relation.id); err != nil {
			return fmt.Errorf("delete purged relation metadata: %w", err)
		}
		for _, pair := range []struct {
			table, field pgtype.UUID
		}{{relation.parentTable, relation.parentField}, {relation.childTable, relation.childField}} {
			if !tableSet[pair.table] {
				if err := queries.DeletePurgedFields(ctx, []pgtype.UUID{pair.field}); err != nil {
					return fmt.Errorf("delete orphaned relation field: %w", err)
				}
			}
		}
	}
	return notifyPurgedSchemas(ctx, tx, affected)
}

func deletePurgedRelation(ctx context.Context, queries *structuresql.Queries, relationID string) error {
	var id pgtype.UUID
	if err := id.Scan(relationID); err != nil {
		return fmt.Errorf("parse purged relation ID: %w", err)
	}
	return queries.DeletePurgedRelation(ctx, id)
}

func markPurgedDependencies(ctx context.Context, tx pgx.Tx, tableIDs []pgtype.UUID,
	fieldIDs []string, affected map[pgtype.UUID]bool) error {
	if len(fieldIDs) == 0 {
		return nil
	}
	rows, err := structuresql.New(tx).MarkPurgedDependencies(ctx, structuresql.MarkPurgedDependenciesParams{
		TableIds: tableIDs, FieldIds: fieldIDs,
	})
	if err != nil {
		return fmt.Errorf("mark purged dependencies: %w", err)
	}
	for _, id := range rows {
		affected[id] = true
	}
	return nil
}

func notifyPurgedSchemas(ctx context.Context, tx pgx.Tx, tables map[pgtype.UUID]bool) error {
	queries := structuresql.New(tx)
	for id := range tables {
		version, err := queries.BumpPurgedSchemaVersion(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("bump purged dependency schema: %w", err)
		}
		if err := events.Emit(ctx, tx, events.Event{Type: "schema.changed", TableID: id.String(), SchemaVersion: version}); err != nil {
			return err
		}
	}
	return nil
}

func dropRelationStorage(ctx context.Context, tx pgx.Tx, relation relationRecord) error {
	queries := structuresql.New(tx)
	if relation.fkColumn.Valid {
		table, err := queries.ReadTrashRelationTable(ctx, relation.childTable)
		if err != nil {
			return fmt.Errorf("read relation storage table: %w", err)
		}
		return ddl.DropColumn(ctx, tx, ddl.TableName{Schema: table.PgSchema, Name: table.PgName}, relation.fkColumn.String)
	}
	if relation.junction.Valid {
		schema, err := queries.ReadTrashRelationSchema(ctx, relation.bookID)
		if err != nil {
			return fmt.Errorf("read relation storage schema: %w", err)
		}
		return ddl.DropJunction(ctx, tx, schema, relation.junction.String)
	}
	return errors.New("purged relation has no physical storage")
}

func purgeField(ctx context.Context, tx pgx.Tx, fieldID pgtype.UUID, cutoff time.Time) error {
	queries := structuresql.New(tx)
	field, err := queries.LockPurgedField(ctx, fieldID)
	if err != nil {
		return fmt.Errorf("read purged field: %w", err)
	}
	fieldIDs := []pgtype.UUID{fieldID}
	if field.UiType == "link" {
		relation, err := relationForAnyField(ctx, tx, fieldID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			fieldIDs = []pgtype.UUID{relation.parentField, relation.childField}
			expired, err := queries.CountExpiredRelationFields(ctx, structuresql.CountExpiredRelationFieldsParams{
				Ids: fieldIDs, Cutoff: pgtype.Timestamptz{Time: cutoff, Valid: true},
			})
			if err != nil {
				return fmt.Errorf("check relation retention: %w", err)
			}
			if expired != 2 {
				return errProtectedTrash
			}
			if err := dropRelationStorage(ctx, tx, relation); err != nil {
				return err
			}
			if err := deletePurgedRelation(ctx, queries, relation.id); err != nil {
				return fmt.Errorf("delete purged relation: %w", err)
			}
		}
	} else if field.PgColumn.Valid {
		if err := ddl.DropColumn(ctx, tx, ddl.TableName{Schema: field.PgSchema, Name: field.PgName}, field.PgColumn.String); err != nil {
			return err
		}
	} else if field.UiType != "formula" && !isComputedKind(field.UiType) {
		return errors.New("purged stored field has no physical column")
	}
	if err := queueFieldAttachments(ctx, tx, fieldIDs); err != nil {
		return err
	}
	if err := queries.DeletePurgedFields(ctx, fieldIDs); err != nil {
		return fmt.Errorf("delete purged field metadata: %w", err)
	}
	return nil
}

func (s *Service) drainPurgeFiles(ctx context.Context, files PurgeFileStore) error {
	queued, err := s.queries.ListQueuedAttachmentRemovals(ctx)
	if err != nil {
		return fmt.Errorf("list queued attachment removals: %w", err)
	}
	return s.removeQueuedFiles(ctx, files, queued)
}

func (s *Service) removeQueuedFiles(ctx context.Context, files PurgeFileStore, queued []structuresql.MetaAttachmentPurgeQueue) error {
	var failures []error
	for _, file := range queued {
		if err := files.RemovePurgedFile(ctx, file.ID.String(), file.StoragePath, file.HasThumb); err != nil {
			failures = append(failures, fmt.Errorf("remove attachment %s: %w", file.ID.String(), err))
			continue
		}
		if err := s.queries.AcknowledgeAttachmentRemoval(ctx, structuresql.AcknowledgeAttachmentRemovalParams{
			ID: file.ID, StoragePath: file.StoragePath, HasThumb: file.HasThumb,
		}); err != nil {
			failures = append(failures, fmt.Errorf("acknowledge attachment %s removal: %w", file.ID.String(), err))
		}
	}
	return errors.Join(failures...)
}

// PurgeOwnedWorkspace removes a workspace inside the caller's locked transaction.
func PurgeOwnedWorkspace(ctx context.Context, tx pgx.Tx, workspace, user pgtype.UUID) error {
	if !workspace.Valid || !user.Valid {
		return errors.New("workspace purge requires valid workspace and user IDs")
	}
	return purgeTree(ctx, tx, purgeCandidate{kind: "workspace", id: workspace, workspace: workspace},
		time.Now(), user)
}

// DrainUserDeletionFiles retries durable file removals for one deletion, or all
// user deletions when user is empty. Failed removals stay in the purge queue.
func (s *Service) DrainUserDeletionFiles(ctx context.Context, files PurgeFileStore, user string) error {
	if files == nil {
		return errors.New("user deletion cleanup requires an attachment file store")
	}
	var id pgtype.UUID
	if user != "" {
		if err := id.Scan(user); err != nil || !id.Valid {
			return errors.New("invalid deletion cleanup user ID")
		}
	}
	queued, err := s.queries.ListUserDeletionAttachmentRemovals(ctx, id)
	if err != nil {
		return fmt.Errorf("list user deletion attachments: %w", err)
	}
	return s.removeQueuedFiles(ctx, files, queued)
}

func (s *Service) PendingUserDeletionFiles(ctx context.Context) (int64, error) {
	count, err := s.queries.CountUserDeletionAttachmentRemovals(ctx)
	if err != nil {
		return 0, fmt.Errorf("count user deletion attachments: %w", err)
	}
	return count, nil
}

// PurgeTrash removes expired resources transactionally and drains the durable
// attachment queue. Each failed object or file remains retryable on the next run.
func (s *Service) PurgeTrash(ctx context.Context, now time.Time, files PurgeFileStore) (int, error) {
	if files == nil {
		return 0, errors.New("trash purge requires an attachment file store")
	}
	cutoff, _, err := trashRetention(ctx, s.pool, now)
	if err != nil {
		return 0, err
	}
	candidates, err := s.trashCandidates(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	var failures []error
	purged := 0
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return purged, errors.Join(append(failures, err)...)
		}
		done, err := s.purgeCandidate(ctx, candidate, now)
		if err != nil {
			failures = append(failures, fmt.Errorf("purge %s %s: %w", candidate.kind, candidate.id.String(), err))
		} else if done {
			purged++
		}
	}
	if err := s.drainPurgeFiles(ctx, files); err != nil {
		failures = append(failures, err)
	}
	return purged, errors.Join(failures...)
}

func (s *Service) RunTrashRetentionWorker(ctx context.Context, logger *slog.Logger, files PurgeFileStore) {
	ticker := time.NewTicker(trashRetentionInterval)
	defer ticker.Stop()
	for {
		if _, err := s.PurgeTrash(ctx, time.Now(), files); err != nil && ctx.Err() == nil {
			logger.Error("trash retention failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
