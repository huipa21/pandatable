package structure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"pandatable/internal/acl"
	"pandatable/internal/codec"
)

// ImportAttachmentStore clones stored attachment blobs for package imports and
// removes clones whose import transaction did not commit.
type ImportAttachmentStore interface {
	CloneStoredFile(ctx context.Context, sourceID, storedPath string, hasThumb bool, newID string) (string, error)
	RemovePurgedFile(ctx context.Context, id, storedPath string, hasThumb bool) error
}

// UseAttachmentStore lets a same-instance package import relink attachments.
// Without a store, attachment cells keep their names in the issue report.
func (file *ImportFile) UseAttachmentStore(store ImportAttachmentStore) {
	file.attachments = store
}

type importClonedFile struct {
	id, path string
	thumb    bool
}

func (file *ImportFile) removeClonedFiles(files []importClonedFile) {
	if file.attachments == nil || len(files) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, cloned := range files {
		// Best effort: a leftover clone is an unreferenced file, never a row value.
		_ = file.attachments.RemovePurgedFile(ctx, cloned.id, cloned.path, cloned.thumb)
	}
}

func (file *ImportFile) packageLocation() *time.Location {
	if file.zone == nil {
		return time.UTC
	}
	return file.zone
}

func (file *ImportFile) note(column, value, reason string) {
	file.notes = append(file.notes, ImportIssue{Column: column, Value: value, Reason: reason})
}

// readImportMeta returns nil when the workbook has no _meta sheet. A sheet that
// cannot be read as the v2 contract is reported as a note and ignored. invalid
// marks field rows whose cells are malformed.
func (file *ImportFile) readImportMeta(book *excelize.File) (*XLSXMetadata, []bool) {
	if index, err := book.GetSheetIndex(XLSXMetaSheet); err != nil || index < 0 {
		return nil, nil
	}
	rejected := func(detail string) (*XLSXMetadata, []bool) {
		file.note("", detail, ImportReasonMetaInvalid)
		return nil, nil
	}
	rows, err := book.Rows(XLSXMetaSheet)
	if err != nil {
		return rejected(XLSXMetaSheet)
	}
	defer rows.Close()
	info := make(map[string]string)
	headers := XLSXMetaColumns()
	metadata := &XLSXMetadata{}
	var invalid []bool
	line, sawHeader := 0, false
	for rows.Next() {
		line++
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return rejected(XLSXMetaSheet)
		}
		switch {
		case line < XLSXMetaHeaderRow:
			for index := 0; index+1 < len(cells); index += 2 {
				if key := strings.TrimSpace(cells[index]); key != "" {
					info[key] = cells[index+1]
				}
			}
		case line == XLSXMetaHeaderRow:
			if len(cells) != len(headers) {
				return rejected("header")
			}
			for index, header := range headers {
				if strings.TrimSpace(cells[index]) != header {
					return rejected("header")
				}
			}
			sawHeader = true
		default:
			empty := true
			for _, cell := range cells {
				if strings.TrimSpace(cell) != "" {
					empty = false
				}
			}
			if empty {
				continue
			}
			if len(metadata.Fields) >= maxImportColumns || len(cells) > len(headers) {
				return rejected("fields")
			}
			field, ok := parseImportMetaRow(cells)
			metadata.Fields = append(metadata.Fields, field)
			invalid = append(invalid, !ok)
		}
	}
	if rows.Error() != nil || !sawHeader {
		return rejected(XLSXMetaSheet)
	}
	if info["format_version"] != XLSXFormatVersion {
		return rejected("format_version")
	}
	metadata.FormatVersion = info["format_version"]
	metadata.SourceInstanceID = strings.TrimSpace(info["source_instance_id"])
	metadata.TableID = strings.TrimSpace(info["table_id"])
	metadata.TableName = info["table_name"]
	if version, err := strconv.ParseInt(strings.TrimSpace(info["schema_version"]), 10, 32); err == nil {
		metadata.SchemaVersion = int32(version)
	}
	if exported, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(info["exported_at"])); err == nil {
		metadata.ExportedAt = exported
	}
	metadata.DatetimeTimezone = strings.TrimSpace(info["datetime_timezone"])
	return metadata, invalid
}

func parseImportMetaRow(cells []string) (XLSXMetaField, bool) {
	padded := make([]string, len(XLSXMetaColumns()))
	copy(padded, cells)
	ok := true
	flag := func(value string) bool {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true":
			return true
		case "", "0", "false":
			return false
		}
		ok = false
		return false
	}
	field := XLSXMetaField{
		Column: padded[0], FieldID: padded[1], Name: padded[2], UIType: padded[3], PGType: padded[4],
		Required: flag(padded[5]), Unique: flag(padded[6]), Default: padded[7], OptionsJSON: padded[8],
		RelationTargetTableID: padded[9], RelationTargetTableName: padded[10], RelationDisplayField: padded[11],
		RelationCardinality: padded[12], AttachmentMode: padded[13], FormulaOrExpression: padded[14],
		Description: padded[15], System: flag(padded[16]),
	}
	return field, ok
}

// applyImportMetaSource records workbook-level settings shared by both the
// merged and the metadata-only package paths.
func (file *ImportFile) applyImportMetaSource(metadata *XLSXMetadata) {
	file.Analysis.MetaVersion = "v2"
	if id, ok := acl.ParseUUID(metadata.SourceInstanceID); ok && id.String() == metadata.SourceInstanceID {
		file.sourceInstanceID = metadata.SourceInstanceID
	}
	if metadata.DatetimeTimezone != "" && metadata.DatetimeTimezone != "UTC" {
		zone, err := codec.LoadLocation(metadata.DatetimeTimezone)
		if err != nil {
			file.note("", "datetime_timezone", ImportReasonMetaInvalid)
		} else {
			file.zone = zone
		}
	}
	file.fieldSources = make(map[string]string)
	file.relationTargets = make(map[string]string)
}

// mergeImportMeta applies _meta field rows over the v1 manifest. _meta wins
// for writable field types; link, formula and computed fields keep their
// manifest options and gain the relation target id. An invalid row keeps the
// manifest definition for that column only.
func (file *ImportFile) mergeImportMeta(metadata *XLSXMetadata, invalid []bool) {
	file.applyImportMetaSource(metadata)
	manifest := file.manifest
	if hash, err := XLSXMetaSHA256(metadata.Fields); err != nil || hash != manifest.MetaSHA256 {
		file.note("", XLSXMetaSheet, ImportReasonMetaEdited)
	}
	if len(metadata.Fields) != len(manifest.Fields) {
		file.note("", "fields", ImportReasonMetaInvalid)
	}
	for index, field := range manifest.Fields {
		if index >= len(metadata.Fields) || invalid[index] || metadata.Fields[index].Name != field.Name {
			file.fieldSources[field.Key] = "manifest"
			if index < len(metadata.Fields) {
				file.note(field.Name, metadata.Fields[index].UIType, ImportReasonMetaInvalid)
			}
			continue
		}
		row := metadata.Fields[index]
		if packageStructuralKind(field.UIType) || packageStructuralKind(row.UIType) {
			if row.UIType != field.UIType {
				file.fieldSources[field.Key] = "manifest"
				file.note(field.Name, row.UIType, ImportReasonMetaInvalid)
				continue
			}
			if field.UIType == "link" && row.RelationTargetTableID != "" {
				file.relationTargets[field.Key] = row.RelationTargetTableID
			}
			file.fieldSources[field.Key] = "meta"
			continue
		}
		patched, err := importMetaUserField(field, row)
		if err != nil {
			file.fieldSources[field.Key] = "manifest"
			file.note(field.Name, row.UIType, ImportReasonMetaInvalid)
			continue
		}
		manifest.Fields[index] = patched
		file.fieldSources[field.Key] = "meta"
	}
}

// packageStructuralKind reports kinds restored from manifest structure rather
// than from editable _meta options.
func packageStructuralKind(kind string) bool {
	return codec.IsSystem(codec.Kind(kind)) || kind == "link" || kind == "formula" || isComputedKind(kind)
}

// importMetaUserField validates one writable _meta row through the field
// codecs; pg_type is informational and recomputed from the bound options.
func importMetaUserField(base packageField, row XLSXMetaField) (packageField, error) {
	options := json.RawMessage(strings.TrimSpace(row.OptionsJSON))
	if len(options) == 0 {
		options = json.RawMessage(`{}`)
	}
	value := json.RawMessage(strings.TrimSpace(row.Default))
	if len(value) == 0 {
		value = json.RawMessage(`null`)
	}
	if !json.Valid(options) || !json.Valid(value) {
		return packageField{}, validation("The metadata row is not valid JSON")
	}
	bound, err := bindUserKind(row.UIType, options)
	if err != nil {
		return packageField{}, err
	}
	resolved, err := resolveDefault(bound, value)
	if err != nil {
		return packageField{}, err
	}
	if err := checkConstraints(bound, row.Required, row.Unique, resolved.stored != nil); err != nil {
		return packageField{}, err
	}
	base.UIType, base.Options = row.UIType, bound.Options()
	base.PGType = pgTypeName(bound.Column().Type)
	base.Required, base.Unique = row.Required, row.Unique
	base.Default = value
	return base, nil
}

// synthesizeImportManifest rebuilds a package manifest from _meta when the
// _pandatable sheet is missing. Rows that cannot be restored are inferred from
// their data. It returns false when the rows cannot describe a package table.
func (file *ImportFile) synthesizeImportManifest(metadata *XLSXMetadata, invalid []bool) bool {
	if len(metadata.Fields) == 0 {
		file.note("", "fields", ImportReasonMetaInvalid)
		return false
	}
	manifest := &tablePackageManifest{Format: "pandatable.table", FormatVersion: 1, RowIDColumn: true,
		Fields: make([]packageField, 0, len(metadata.Fields))}
	if name, err := cleanName(metadata.TableName); err == nil {
		manifest.Table.Name = name
	}
	keys := make(map[string]string, len(metadata.Fields))
	names := make(map[string]bool, len(metadata.Fields))
	systems := make(map[string]bool)
	for index, row := range metadata.Fields {
		name, err := cleanName(row.Name)
		if err != nil || name != row.Name || names[strings.ToLower(name)] {
			file.note(row.Name, "name", ImportReasonMetaInvalid)
			return false
		}
		names[strings.ToLower(name)] = true
		if row.FieldID != "" {
			keys[row.FieldID] = fmt.Sprintf("f%d", index+1)
		}
		if codec.IsSystem(codec.Kind(row.UIType)) {
			valid := !systems[row.UIType] && !invalid[index] && !row.Unique
			for _, system := range systemFields {
				if system.column == row.UIType && system.required != row.Required {
					valid = false
				}
			}
			if !valid {
				file.note(row.Name, row.UIType, ImportReasonMetaInvalid)
				return false
			}
			systems[row.UIType] = true
		}
	}
	if len(systems) != len(systemFields) {
		file.note("", "system", ImportReasonMetaInvalid)
		return false
	}
	file.applyImportMetaSource(metadata)
	file.inferKeys = make(map[string]bool)
	file.note("", "_pandatable", ImportReasonMetaEdited)
	for index, row := range metadata.Fields {
		field := packageField{Key: fmt.Sprintf("f%d", index+1), Name: row.Name, UIType: row.UIType,
			Options: json.RawMessage(`{}`), Required: row.Required, Default: json.RawMessage(`null`)}
		ok := !invalid[index]
		switch {
		case !ok:
		case codec.IsSystem(codec.Kind(row.UIType)):
		case row.UIType == "link":
			field.Options, ok = importMetaLinkOptions(row)
			if ok && row.RelationTargetTableID != "" {
				file.relationTargets[field.Key] = row.RelationTargetTableID
			}
		case row.UIType == "formula":
			field.Options, ok = importMetaFormulaOptions(row)
		case isComputedKind(row.UIType):
			field.Options, ok = importMetaComputedOptions(row, keys, metadata.Fields)
		default:
			field, ok = importMetaUserFieldOK(field, row)
		}
		if !ok {
			field.UIType, field.Options = string(codec.Text), json.RawMessage(`{}`)
			field.Required, field.Unique, field.Default = false, false, json.RawMessage(`null`)
			file.inferKeys[field.Key] = true
			file.fieldSources[field.Key] = "inferred"
			file.note(row.Name, row.UIType, ImportReasonMetaInvalid)
		} else {
			file.fieldSources[field.Key] = "meta"
		}
		manifest.Fields = append(manifest.Fields, field)
	}
	file.manifest = manifest
	file.metaOnly = true
	return true
}

func importMetaUserFieldOK(base packageField, row XLSXMetaField) (packageField, bool) {
	patched, err := importMetaUserField(base, row)
	if err != nil {
		return base, false
	}
	return patched, true
}

func importMetaLinkOptions(row XLSXMetaField) (json.RawMessage, bool) {
	var options struct {
		Kind        string  `json:"kind"`
		Side        string  `json:"side"`
		TargetTable *string `json:"targetTable"`
		OnDelete    string  `json:"onDelete"`
	}
	if json.Unmarshal([]byte(row.OptionsJSON), &options) != nil || options.Kind == "" ||
		options.Side != "parent" && options.Side != "child" {
		return nil, false
	}
	if options.TargetTable == nil && row.RelationTargetTableName != "" {
		name := row.RelationTargetTableName
		options.TargetTable = &name
	}
	encoded, err := json.Marshal(options)
	return encoded, err == nil
}

func importMetaFormulaOptions(row XLSXMetaField) (json.RawMessage, bool) {
	expression := row.FormulaOrExpression
	if expression == "" {
		var options struct {
			Expression string `json:"expr"`
		}
		if json.Unmarshal([]byte(row.OptionsJSON), &options) != nil {
			return nil, false
		}
		expression = options.Expression
	}
	if strings.TrimSpace(expression) == "" {
		return nil, false
	}
	encoded, err := json.Marshal(struct {
		Expression string `json:"expr"`
	}{Expression: expression})
	return encoded, err == nil
}

// importMetaComputedOptions converts readable computed options, which name
// source field ids, to package keys. The target field is restored only from a
// package-form target name; otherwise the field is marked dependency_missing.
func importMetaComputedOptions(row XLSXMetaField, keys map[string]string, fields []XLSXMetaField) (json.RawMessage, bool) {
	options := make(map[string]json.RawMessage)
	if json.Unmarshal([]byte(row.OptionsJSON), &options) != nil || options == nil {
		return nil, false
	}
	var relation string
	if json.Unmarshal(options["relationFieldId"], &relation) != nil {
		return nil, false
	}
	key, ok := keys[relation]
	if !ok {
		for index, field := range fields {
			if fmt.Sprintf("f%d", index+1) == relation && field.UIType == "link" {
				key, ok = relation, true
			}
		}
	}
	if !ok {
		return nil, false
	}
	for index, field := range fields {
		if fmt.Sprintf("f%d", index+1) == key && field.UIType != "link" {
			return nil, false
		}
	}
	encoded, err := json.Marshal(key)
	if err != nil {
		return nil, false
	}
	options["relationFieldId"] = encoded
	delete(options, "targetFieldId")
	result, err := json.Marshal(options)
	return result, err == nil
}

// createPackageDefaultView gives a metadata-only package, which has no saved
// views, the same default grid as a plain import.
func createPackageDefaultView(ctx context.Context, c *txn, table rowTable, manifest *tablePackageManifest,
	keys map[string]string, systemIDs []string,
) (string, error) {
	var display pgtype.UUID
	if manifest.Table.DisplayField != nil {
		display, _ = acl.ParseUUID(keys[*manifest.Table.DisplayField])
	}
	if !display.Valid {
		for _, field := range manifest.Fields {
			created, ok := table.byID[keys[field.Key]]
			if ok && created.relation == nil && created.computed == nil && created.formula == nil &&
				displayEligible(string(created.kind), created.system) {
				display, _ = acl.ParseUUID(created.id)
				break
			}
		}
	}
	if !display.Valid {
		return "", validation("An imported table needs a displayable field")
	}
	viewID, err := createImportDefaultView(ctx, c, table.id, display, systemIDs)
	if err != nil {
		return "", err
	}
	return viewID.String(), nil
}

type importAttachmentCell struct {
	row, position, number int
	column                importColumnPlan
	text                  string
}

type importSourceAttachment struct {
	table    pgtype.UUID
	name     string
	mime     string
	size     int64
	sha256   []byte
	path     string
	hasThumb bool
}

// relinkImportAttachments recreates package attachment references for the
// imported cells. A reference is relinked only when the workbook comes from
// this instance, the attachment still exists and the importer can read its
// table; the new attachment shares (or copies) the stored blob. Anything else
// keeps its file name in a file_unavailable issue.
func (s *Service) relinkImportAttachments(ctx context.Context, tx pgx.Tx, actor Actor, file *ImportFile,
	table rowTable, values [][]any, cells []importAttachmentCell, result *ImportResult,
) error {
	if len(cells) == 0 {
		return nil
	}
	user, ok := acl.ParseUUID(actor.ID)
	if !ok {
		return acl.ErrValidation()
	}
	parsed := make([][]XLSXAttachmentReference, len(cells))
	malformed := make([]bool, len(cells))
	wanted := make(map[pgtype.UUID]bool)
	for index, cell := range cells {
		if json.Unmarshal([]byte(cell.text), &parsed[index]) != nil {
			malformed[index] = true
			continue
		}
		for _, reference := range parsed[index] {
			if id, valid := acl.ParseUUID(reference.ID); valid && id.String() == reference.ID {
				wanted[id] = true
			}
		}
	}
	sources := make(map[string]importSourceAttachment)
	if file != nil && file.attachments != nil && file.sourceInstanceID != "" && len(wanted) > 0 {
		instance, err := s.xlsxSourceInstanceID(ctx)
		if err != nil {
			return err
		}
		if instance == file.sourceInstanceID {
			sources, err = readableImportAttachments(ctx, tx, user, wanted)
			if err != nil {
				return err
			}
		}
	}
	for index, cell := range cells {
		var missing []string
		if malformed[index] {
			missing = append(missing, cell.text)
		}
		var options codec.AttachmentOptions
		if err := json.Unmarshal(cell.column.field.codec.Options(), &options); err != nil {
			return fmt.Errorf("read import attachment options: %w", err)
		}
		if len(parsed[index]) > options.MaxFiles {
			result.issue(cell.number, cell.column.index, cell.column.name, cell.text)
			continue
		}
		fieldID, ok := acl.ParseUUID(cell.column.field.id)
		if !ok {
			return errors.New("import attachment field has an invalid id")
		}
		linked := make([]codec.AttachmentValue, 0, len(parsed[index]))
		for _, reference := range parsed[index] {
			source, found := sources[reference.ID]
			label := reference.Name
			if label == "" {
				label = reference.ID
			}
			if !found {
				missing = append(missing, label)
				continue
			}
			var newID pgtype.UUID
			if err := tx.QueryRow(ctx, "SELECT uuidv7()").Scan(&newID); err != nil {
				return fmt.Errorf("allocate import attachment id: %w", err)
			}
			path, err := file.attachments.CloneStoredFile(ctx, reference.ID, source.path, source.hasThumb, newID.String())
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, label)
				continue
			}
			if err != nil {
				return err
			}
			result.files = append(result.files, importClonedFile{id: newID.String(), path: path, thumb: source.hasThumb})
			if _, err := tx.Exec(ctx, `INSERT INTO meta.attachments (id, table_id, field_id, file_name, mime_type,
				size_bytes, sha256, storage_path, has_thumb, uploaded_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				newID, table.id, fieldID, source.name, source.mime, source.size, source.sha256, path,
				source.hasThumb, user); err != nil {
				return fmt.Errorf("insert import attachment: %w", err)
			}
			value := codec.AttachmentValue{ID: newID.String(), Name: source.name, MIME: source.mime, Size: source.size}
			if source.hasThumb {
				thumb := codec.ThumbnailURL(value.ID)
				value.Thumb = &thumb
			}
			linked = append(linked, value)
		}
		if len(linked) > 0 {
			raw, err := json.Marshal(linked)
			if err != nil {
				return fmt.Errorf("encode import attachments: %w", err)
			}
			value, err := cell.column.field.codec.Decode(raw)
			if err != nil {
				return fmt.Errorf("validate import attachments: %w", err)
			}
			values[cell.row][cell.position] = value
		}
		if len(missing) > 0 {
			result.issueReason(cell.number, cell.column.index, cell.column.name, strings.Join(missing, ", "),
				ImportReasonFileUnavailable)
		}
	}
	return nil
}

// readableImportAttachments loads live attachments whose table the user can
// read. Unreadable and missing attachments are indistinguishable.
func readableImportAttachments(ctx context.Context, tx pgx.Tx, user pgtype.UUID, wanted map[pgtype.UUID]bool,
) (map[string]importSourceAttachment, error) {
	ids := make([]pgtype.UUID, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.table_id, a.file_name, a.mime_type, a.size_bytes, a.sha256,
			a.storage_path, a.has_thumb
		FROM meta.attachments a
		JOIN meta.tables t ON t.id = a.table_id AND t.deleted_at IS NULL
		JOIN meta.fields f ON f.id = a.field_id AND f.table_id = a.table_id AND f.deleted_at IS NULL
		WHERE a.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, fmt.Errorf("read import attachment sources: %w", err)
	}
	sources := make(map[string]importSourceAttachment)
	for rows.Next() {
		var id pgtype.UUID
		var source importSourceAttachment
		if err := rows.Scan(&id, &source.table, &source.name, &source.mime, &source.size, &source.sha256,
			&source.path, &source.hasThumb); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read import attachment source: %w", err)
		}
		sources[id.String()] = source
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate import attachment sources: %w", err)
	}
	readable := make(map[pgtype.UUID]bool)
	for id, source := range sources {
		allowed, checked := readable[source.table]
		if !checked {
			access, err := acl.AccessUncached(ctx, tx, user, acl.Resource{Type: acl.Table, ID: source.table})
			if err != nil {
				return nil, err
			}
			allowed = access.Visible && access.Check(acl.Read) == nil
			readable[source.table] = allowed
		}
		if !allowed {
			delete(sources, id)
		}
	}
	return sources, nil
}
