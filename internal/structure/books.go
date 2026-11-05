package structure

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
	"pandatable/internal/ddl"
	"pandatable/internal/structure/structuresql"
)

type BookPatch struct {
	Name  *string
	Index *int
	Icon  OptionalText
}

func parentWorkspace(path acl.Path) acl.Resource {
	return acl.Resource{Type: acl.Workspace, ID: path.Workspace}
}

// reorder applies one deterministic move among live siblings under the parent
// lock the caller already holds.
func reorder(siblings []sibling, moving pgtype.UUID, index int,
	setPosition func(float64) error, renumber func([]pgtype.UUID) error) error {
	moved, err := place(siblings, moving, index)
	if err != nil {
		return err
	}
	switch {
	case moved.Unchanged:
		return nil
	case moved.Renumber != nil:
		return renumber(moved.Renumber)
	default:
		return setPosition(moved.Position)
	}
}

// CreateBook creates a generated PostgreSQL schema and its metadata in one
// transaction. Only an owner of the live workspace may do so.
func (s *Service) CreateBook(ctx context.Context, actor Actor, workspaceID, name string) (Book, error) {
	return s.CreateBookWithIcon(ctx, actor, workspaceID, name, nil)
}

// CreateBookWithIcon creates a book with an optional user-selected icon.
func (s *Service) CreateBookWithIcon(ctx context.Context, actor Actor, workspaceID, name string, icon *string) (Book, error) {
	target, err := resourceOf(acl.Workspace, workspaceID)
	if err != nil {
		return Book{}, err
	}
	cleaned, err := cleanName(name)
	if err != nil {
		return Book{}, err
	}
	customIcon, err := cleanOptional(icon, cleanBookIcon)
	if err != nil {
		return Book{}, err
	}
	var created Book
	err = retryCollisions(func() error {
		return s.run(ctx, actor, scope{target: target, affectsACL: true}, func(c *txn) error {
			schema, err := s.newName(ddl.Book)
			if err != nil {
				return err
			}
			last, err := c.q.MaxBookPosition(ctx, target.ID)
			if err != nil {
				return fmt.Errorf("read book position: %w", err)
			}
			if err := ddl.CreateSchema(ctx, c.tx, schema); err != nil {
				return err
			}
			row, err := c.q.InsertBook(ctx, structuresql.InsertBookParams{WorkspaceID: target.ID, Name: cleaned,
				Icon: textValue(customIcon), PgSchema: schema, Position: last + positionStep, CreatedBy: c.user})
			if err != nil {
				return fmt.Errorf("insert book: %w", err)
			}
			created = bookDTO(row.ID, row.WorkspaceID, row.Name, row.Icon, row.FallbackIcon, row.Position, row.CreatedAt)
			return nil
		})
	})
	if err != nil {
		return Book{}, err
	}
	return created, nil
}

// UpdateBook renames (metadata only) and/or moves a book among its live
// siblings. Index is the zero-based target position among the other siblings.
func (s *Service) UpdateBook(ctx context.Context, actor Actor, id string, patch BookPatch) (Book, error) {
	target, err := resourceOf(acl.Book, id)
	if err != nil {
		return Book{}, err
	}
	if patch.Name == nil && patch.Index == nil && !patch.Icon.Set {
		return Book{}, validation("At least one book field is required")
	}
	var name string
	if patch.Name != nil {
		if name, err = cleanName(*patch.Name); err != nil {
			return Book{}, err
		}
	}
	icon, err := cleanOptional(patch.Icon.Value, cleanBookIcon)
	if err != nil {
		return Book{}, err
	}
	if patch.Index != nil && *patch.Index < 0 {
		return Book{}, validation("Index is outside the sibling range")
	}
	var updated Book
	err = s.run(ctx, actor, scope{target: target, owner: parentWorkspace}, func(c *txn) error {
		if patch.Name != nil {
			if _, err := c.q.RenameBook(ctx, structuresql.RenameBookParams{ID: target.ID, Name: name}); err != nil {
				return notFoundIfNoRows(err, "rename book")
			}
		}
		if patch.Icon.Set {
			if _, err := c.q.SetBookIcon(ctx, structuresql.SetBookIconParams{ID: target.ID, Icon: textValue(icon)}); err != nil {
				return notFoundIfNoRows(err, "update book icon")
			}
		}
		if patch.Index != nil {
			rows, err := c.q.ListLiveBookOrder(ctx, c.path.Workspace)
			if err != nil {
				return fmt.Errorf("list book order: %w", err)
			}
			siblings := make([]sibling, len(rows))
			for index, row := range rows {
				siblings[index] = sibling{ID: row.ID, Position: row.Position}
			}
			if err := reorder(siblings, target.ID, *patch.Index, func(position float64) error {
				_, err := c.q.SetBookPosition(ctx, structuresql.SetBookPositionParams{ID: target.ID, Position: position})
				return notFoundIfNoRows(err, "move book")
			}, func(order []pgtype.UUID) error {
				return c.q.RenumberBooks(ctx, order)
			}); err != nil {
				return err
			}
		}
		row, err := c.q.GetBook(ctx, target.ID)
		if err != nil {
			return notFoundIfNoRows(err, "read book")
		}
		updated = bookDTO(row.ID, row.WorkspaceID, row.Name, row.Icon, row.FallbackIcon, row.Position, row.CreatedAt)
		return nil
	})
	return updated, err
}

// DeleteBook soft-deletes a book. Its schema, tables, data and grants are
// retained; the ACL hides the book and its tables at once.
func (s *Service) DeleteBook(ctx context.Context, actor Actor, id string) error {
	target, err := resourceOf(acl.Book, id)
	if err != nil {
		return err
	}
	return s.run(ctx, actor, scope{target: target, owner: parentWorkspace, affectsACL: true}, func(c *txn) error {
		if _, err := c.q.TrashBook(ctx, structuresql.TrashBookParams{ID: target.ID, DeletedBy: c.user}); err != nil {
			return notFoundIfNoRows(err, "trash book")
		}
		return nil
	})
}
