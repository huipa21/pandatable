package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"pandatable/internal/acl"
)

type importLinkPlan struct {
	index  int
	name   string
	field  rowField
	source ImportColumn
}

func (s *Service) createPackageRelation(ctx context.Context, c *txn, sourceID pgtype.UUID, tableName string,
	field packageField, column ImportColumn,
) (pgtype.UUID, error) {
	if column.RelinkTargetID == "" {
		return pgtype.UUID{}, validation("The package link target cannot be re-linked")
	}
	target, err := resourceOf(acl.Table, column.RelinkTargetID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	targetMeta, err := c.q.GetTable(ctx, target.ID)
	if err != nil {
		return pgtype.UUID{}, notFoundIfNoRows(err, "read package link target")
	}
	if !strings.EqualFold(targetMeta.Name, column.LinkTarget) {
		return pgtype.UUID{}, validation("The package link target changed; analyze the file again")
	}
	targetFields, err := c.q.ListTableFields(ctx, target.ID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("read package relation target fields: %w", err)
	}
	var options struct {
		Kind     string `json:"kind"`
		Side     string `json:"side"`
		OnDelete string `json:"onDelete"`
	}
	if err := json.Unmarshal(field.Options, &options); err != nil {
		return pgtype.UUID{}, validation("The package link options are invalid")
	}
	input := RelationInput{Name: field.Name, ReverseName: packageReverseName(tableName, field.Name, targetFields),
		TargetTableID: target.ID.String(), Kind: options.Kind, SourceSide: options.Side, OnDelete: options.OnDelete}
	validate := input
	if validate.Kind == "many_many" && validate.SourceSide == "child" {
		validate.SourceSide = "parent"
	}
	name, reverse, onDelete, err := cleanRelationInput(validate)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if options.Side != "parent" && options.Side != "child" {
		return pgtype.UUID{}, validation("The package link side is invalid")
	}
	source := acl.Resource{Type: acl.Table, ID: sourceID}
	var result RelationResult
	if err := s.createRelationTx(ctx, c, source, target, input, name, reverse, onDelete, options.Side, &result); err != nil {
		return pgtype.UUID{}, err
	}
	if result.Field == nil {
		return pgtype.UUID{}, errors.New("created package relation field is unavailable")
	}
	id, ok := acl.ParseUUID(result.Field.ID)
	if !ok {
		return pgtype.UUID{}, errors.New("created package relation field id is invalid")
	}
	return id, nil
}

func (s *Service) applyImportLinks(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable,
	links []importLinkPlan, batch []importRecord, ids []int64, result *ImportResult,
) error {
	if len(links) == 0 {
		return nil
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	for _, column := range links {
		relation := column.field.relation
		if relation == nil {
			return errors.New("package link field has no relation")
		}
		targetID := relation.parentTable
		if column.field.relationSide == "parent" {
			targetID = relation.childTable
		}
		if column.source.RelinkTargetID == "" || column.source.RelinkTargetID != targetID.String() {
			return validation("The selected link target does not match the package")
		}
		target := acl.Resource{Type: acl.Table, ID: targetID}
		if _, err := acl.LockPath(ctx, tx, target); err != nil {
			return err
		}
		if err := s.authorizeRows(ctx, tx, user, target, acl.Write); err != nil {
			return err
		}
		targetTable, err := relationRowTable(ctx, tx, targetID)
		if err != nil {
			return err
		}
		labelsByRow := make([][]string, len(batch))
		inputs := make([]string, len(batch))
		invalid := make([]bool, len(batch))
		wanted := make(map[string]bool)
		for index, record := range batch {
			if column.index < len(record.values) {
				inputs[index] = record.values[column.index]
			}
			if strings.TrimSpace(inputs[index]) == "" {
				continue
			}
			if err := json.Unmarshal([]byte(inputs[index]), &labelsByRow[index]); err != nil {
				invalid[index] = true
				continue
			}
			seen := make(map[string]bool, len(labelsByRow[index]))
			for _, label := range labelsByRow[index] {
				if seen[label] {
					invalid[index] = true
				}
				seen[label] = true
				wanted[label] = true
			}
			if relation.kind == "one_one" || relation.kind == "one_many" && column.field.relationSide == "child" {
				if len(labelsByRow[index]) > 1 {
					invalid[index] = true
				}
			}
		}
		matches, err := findImportLinkMatches(ctx, tx, targetID, wanted)
		if err != nil {
			return err
		}
		resolved := make([][]int64, len(batch))
		var targetRows []int64
		for index, labels := range labelsByRow {
			for _, label := range labels {
				idsForLabel := matches[label]
				if len(idsForLabel) != 1 {
					invalid[index] = true
					continue
				}
				resolved[index] = append(resolved[index], idsForLabel[0])
			}
			if invalid[index] {
				result.issue(batch[index].number, column.index, column.name, inputs[index])
				resolved[index] = nil
			} else {
				targetRows = append(targetRows, resolved[index]...)
			}
		}
		if err := lockRelationRows(ctx, tx, targetTable, uniqueSortedIDs(targetRows)); err != nil {
			return err
		}
		var edges []relationEdge
		for index, targetIDs := range resolved {
			if len(targetIDs) == 0 {
				continue
			}
			switch relation.kind {
			case "many_many":
				err = changeJunctionLinks(ctx, tx, *relation, column.field.relationSide, ids[index], targetIDs, nil, user, &edges)
			case "one_one":
				err = changeOneToOneLinks(ctx, tx, table, targetTable, *relation, column.field.relationSide,
					ids[index], targetIDs, nil, false, user, &edges)
			case "one_many":
				err = changeOneToManyLinks(ctx, tx, table, *relation, column.field.relationSide,
					ids[index], targetIDs, nil, user, &edges)
			default:
				return errors.New("package relation kind is invalid")
			}
			if err != nil {
				return err
			}
		}
		if err := recordRelationHistory(ctx, tx, actor, *relation, edges); err != nil {
			return err
		}
		if err := emitRelatedRowInvalidation(ctx, tx, targetID, table.id); err != nil {
			return err
		}
	}
	return nil
}

func findImportLinkMatches(ctx context.Context, tx pgx.Tx, targetID pgtype.UUID, wanted map[string]bool) (map[string][]int64, error) {
	result := make(map[string][]int64)
	if len(wanted) == 0 {
		return result, nil
	}
	table, column, err := relationDisplayColumn(ctx, tx, targetID)
	if err != nil {
		return nil, err
	}
	name, err := relationTableName(table)
	if err != nil {
		return nil, err
	}
	labels := make([]string, 0, len(wanted))
	for label := range wanted {
		labels = append(labels, label)
	}
	expression := relationLabelExpression(`"target"`, column)
	query := `SELECT "target"."id", ` + expression + " FROM " + name +
		` AS "target" WHERE ` + expression + `=ANY($1::text[]) ORDER BY "target"."id"`
	rows, err := tx.Query(ctx, query, labels)
	if err != nil {
		return nil, fmt.Errorf("match package relation labels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("read package relation match: %w", err)
		}
		result[label] = append(result[label], id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package relation matches: %w", err)
	}
	return result, nil
}
