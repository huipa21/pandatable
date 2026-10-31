package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"pandatable/internal/acl"
)

type relationEdge struct {
	parent int64
	child  int64
	action string
}

func relationHistoryLabels(ctx context.Context, tx pgx.Tx, tableID string, ids []int64) (map[int64]string, error) {
	uuid, ok := acl.ParseUUID(tableID)
	if !ok {
		return nil, errors.New("relation history has an invalid table id")
	}
	table, column, err := relationDisplayColumn(ctx, tx, uuid)
	if err != nil {
		return nil, err
	}
	name, err := relationTableName(table)
	if err != nil {
		return nil, err
	}
	query := `SELECT "target"."id", ` + relationLabelExpression("target", column) +
		` FROM ` + name + ` AS "target" WHERE "target"."id"=ANY($1::bigint[])`
	rows, err := tx.Query(ctx, query, uniqueSortedIDs(ids))
	if err != nil {
		return nil, fmt.Errorf("read relation history labels: %w", err)
	}
	defer rows.Close()
	labels := make(map[int64]string, len(ids))
	for rows.Next() {
		var id int64
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("scan relation history label: %w", err)
		}
		labels[id] = label
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relation history labels: %w", err)
	}
	return labels, nil
}

func relationHistoryChange(action string, links []LinkValue) (HistoryChange, error) {
	value, err := json.Marshal(links)
	if err != nil {
		return HistoryChange{}, fmt.Errorf("encode linked row history: %w", err)
	}
	switch action {
	case "link":
		return HistoryChange{New: value}, nil
	case "unlink":
		return HistoryChange{Old: value}, nil
	default:
		return HistoryChange{}, errors.New("invalid relation history action")
	}
}

func recordRelationHistory(ctx context.Context, tx pgx.Tx, actor Actor, relation relationRecord, edges []relationEdge) error {
	if len(edges) == 0 {
		return nil
	}
	parents, children := make([]int64, 0, len(edges)), make([]int64, 0, len(edges))
	for _, edge := range edges {
		parents = append(parents, edge.parent)
		children = append(children, edge.child)
	}
	parentLabels, err := relationHistoryLabels(ctx, tx, relation.parentTable.String(), parents)
	if err != nil {
		return err
	}
	childLabels, err := relationHistoryLabels(ctx, tx, relation.childTable.String(), children)
	if err != nil {
		return err
	}
	type historyKey struct {
		tableID string
		rowID   int64
		action  string
	}
	type groupedHistory struct {
		key    historyKey
		fields map[string][]LinkValue
	}
	groups := make([]groupedHistory, 0, len(edges)*2)
	byKey := make(map[historyKey]int)
	add := func(key historyKey, fieldID string, target int64, label string) {
		index, ok := byKey[key]
		if !ok {
			index = len(groups)
			byKey[key] = index
			groups = append(groups, groupedHistory{key: key, fields: make(map[string][]LinkValue)})
		}
		groups[index].fields[fieldID] = append(groups[index].fields[fieldID],
			LinkValue{ID: strconv.FormatInt(target, 10), Label: label})
	}
	for _, edge := range edges {
		parentLabel, parentFound := parentLabels[edge.parent]
		childLabel, childFound := childLabels[edge.child]
		if !parentFound || !childFound {
			return errors.New("linked row history is missing a display value")
		}
		add(historyKey{relation.parentTable.String(), edge.parent, edge.action},
			relation.parentField.String(), edge.child, childLabel)
		add(historyKey{relation.childTable.String(), edge.child, edge.action},
			relation.childField.String(), edge.parent, parentLabel)
	}
	entries := make([]historyWrite, 0, len(groups))
	for _, group := range groups {
		changes := make(map[string]HistoryChange, len(group.fields))
		for fieldID, links := range group.fields {
			change, err := relationHistoryChange(group.key.action, links)
			if err != nil {
				return err
			}
			changes[fieldID] = change
		}
		entries = append(entries, historyWrite{
			TableID: group.key.tableID, RowID: group.key.rowID, UserID: actor.ID,
			Action: group.key.action, Changes: changes,
		})
	}
	return insertHistory(ctx, tx, entries)
}

type deleteHistoryState struct {
	visited   map[string]map[int64]bool
	snapshots map[string]map[string]RowRecord
	edges     map[string]bool
}

func newDeleteHistoryState() *deleteHistoryState {
	return &deleteHistoryState{
		visited: make(map[string]map[int64]bool), snapshots: make(map[string]map[string]RowRecord),
		edges: make(map[string]bool),
	}
}

func (state *deleteHistoryState) remember(ctx context.Context, tx pgx.Tx, actor Actor, table rowTable, ids []int64) error {
	query, args, err := sq.Select(table.selectColumnList(table.fields)...).
		From(table.qualifiedName()).
		Where(sq.Expr(`"id" = ANY(?::bigint[])`, ids)).
		OrderBy(`"id" ASC`).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build history delete snapshot query: %w", err)
	}
	rows, err := queryUncachedRows(ctx, tx, query, args...)
	if err != nil {
		return fmt.Errorf("read history delete snapshots: %w", err)
	}
	records, err := scanRows(rows, table, table.fields)
	if err != nil {
		return err
	}
	if len(records) != len(ids) {
		return acl.ErrNotFound()
	}
	if err := enrichRowLinks(ctx, tx, table, table.fields, records); err != nil {
		return err
	}
	tableID := table.id.String()
	if state.snapshots[tableID] == nil {
		state.snapshots[tableID] = make(map[string]RowRecord)
	}
	type relationChanges struct {
		relation relationRecord
		edges    []relationEdge
	}
	groups := make(map[string]*relationChanges)
	for _, record := range records {
		state.snapshots[tableID][record.ID] = record
		source, err := canonicalRowID(record.ID)
		if err != nil {
			return err
		}
		for _, field := range table.fields {
			if field.relation == nil {
				continue
			}
			var links []LinkValue
			if err := json.Unmarshal(record.Fields[field.id], &links); err != nil {
				return fmt.Errorf("decode delete history links: %w", err)
			}
			relation := *field.relation
			for _, link := range links {
				target, err := canonicalRowID(link.ID)
				if err != nil {
					return err
				}
				parent, child := source, target
				if field.relationSide == "child" {
					parent, child = target, source
				}
				key := fmt.Sprintf("%s:%d:%d", relation.id, parent, child)
				if state.edges[key] {
					continue
				}
				state.edges[key] = true
				if groups[relation.id] == nil {
					groups[relation.id] = &relationChanges{relation: relation}
				}
				groups[relation.id].edges = append(groups[relation.id].edges,
					relationEdge{parent: parent, child: child, action: "unlink"})
			}
		}
	}
	idsInOrder := make([]string, 0, len(groups))
	for id := range groups {
		idsInOrder = append(idsInOrder, id)
	}
	sort.Strings(idsInOrder)
	for _, id := range idsInOrder {
		group := groups[id]
		if err := recordRelationHistory(ctx, tx, actor, group.relation, group.edges); err != nil {
			return err
		}
	}
	return nil
}

func deletePhysicalRows(ctx context.Context, tx pgx.Tx, table rowTable, ids []int64,
	actor Actor, state *deleteHistoryState) error {
	if len(ids) == 0 {
		return nil
	}
	query := "DELETE FROM " + table.qualifiedName() + ` WHERE "id"=ANY($1::bigint[]) ` +
		"RETURNING WITH (OLD AS history_old) " +
		strings.Join(table.selectColumnListAlias(table.fields, "history_old"), ", ")
	rows, err := queryUncachedRows(ctx, tx, query, ids)
	if err != nil {
		return fmt.Errorf("delete rows: %w", err)
	}
	records, err := scanRows(rows, table, table.fields)
	if err != nil {
		return fmt.Errorf("read deleted rows: %w", err)
	}
	for _, record := range records {
		snapshot, ok := state.snapshots[table.id.String()][record.ID]
		if !ok {
			return errors.New("deleted row history snapshot is missing")
		}
		for _, field := range table.fields {
			if field.relation != nil {
				record.Fields[field.id] = snapshot.Fields[field.id]
			}
		}
	}
	entries, err := snapshotHistories(table, records, actor, "delete")
	if err != nil {
		return err
	}
	return insertHistory(ctx, tx, entries)
}
