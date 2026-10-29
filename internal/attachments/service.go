package attachments

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"pandatable/internal/acl"
	"pandatable/internal/apperr"
	"pandatable/internal/attachments/attachmentssql"
	"pandatable/internal/codec"
	"pandatable/internal/db"
	"pandatable/internal/ddl"
)

const (
	thumbnailSize     = 256
	multipartOverhead = 1 << 20
	maxStoredFileName = 255
	maxSniffBytes     = 512
)

// Service owns local attachment files and their metadata/ACL lifecycle.
type Service struct {
	pool    *db.Pool
	queries *attachmentssql.Queries
	acl     *acl.Resolver
	root    string
}

// File is the attachment-cell value returned after upload.
type File struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	MIME  string  `json:"mime"`
	Size  int64   `json:"size"`
	Thumb *string `json:"thumb"`
}

// OpenedFile holds an ACL-checked file response and its HTTP presentation.
type OpenedFile struct {
	File        File
	Reader      *os.File
	ContentType string
	Inline      bool
	SHA256      string
}

// New configures local attachment storage under dataDir/files.
func New(pool *db.Pool, resolver *acl.Resolver, dataDir string) (*Service, error) {
	if pool == nil || pool.Pool == nil || resolver == nil || strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("attachment service requires a database pool, ACL resolver, and data directory")
	}
	root, err := filepath.Abs(filepath.Join(dataDir, "files"))
	if err != nil {
		return nil, fmt.Errorf("resolve attachment directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("attachment directory must be a non-symlink directory")
	}
	return &Service{pool: pool, queries: attachmentssql.New(pool), acl: resolver, root: root}, nil
}

// Upload streams one file, enforces field and administrator limits, and records metadata.
func (service *Service) Upload(ctx context.Context, userID, tableID, fieldID, fileName string, source io.Reader,
	maxBytes int64, maxMegapixels int) (File, error) {
	tableUUID, ok := acl.ParseUUID(tableID)
	if !ok {
		return File{}, acl.ErrNotFound()
	}
	fieldUUID, ok := acl.ParseUUID(fieldID)
	if !ok || source == nil || maxBytes < 1 || maxMegapixels < 1 || maxMegapixels > 16 {
		return File{}, acl.ErrValidation()
	}
	if err := service.acl.Require(ctx, userID, acl.Write, acl.Resource{Type: acl.Table, ID: tableUUID}); err != nil {
		return File{}, err
	}
	optionsRaw, err := service.queries.FieldForUpload(ctx, attachmentssql.FieldForUploadParams{ID: fieldUUID, TableID: tableUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, acl.ErrNotFound()
	}
	if err != nil {
		return File{}, fmt.Errorf("read attachment field: %w", err)
	}
	bound, err := codec.Bind(codec.Attachment, optionsRaw)
	if err != nil {
		return File{}, fmt.Errorf("bind attachment field: %w", err)
	}
	var options codec.AttachmentOptions
	if err := json.Unmarshal(bound.Options(), &options); err != nil {
		return File{}, fmt.Errorf("decode attachment options: %w", err)
	}
	name, err := cleanName(fileName)
	if err != nil {
		return File{}, err
	}
	id, idText, err := newUUID()
	if err != nil {
		return File{}, fmt.Errorf("generate attachment id: %w", err)
	}
	now := time.Now().UTC()
	year, month := now.Format("2006"), now.Format("01")
	directory, err := service.storageDirectory(year, month)
	if err != nil {
		return File{}, err
	}
	temporary := filepath.Join(directory, ".upload-"+idText)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("create attachment temp file: %w", err)
	}
	keepTemporary := false
	defer func() {
		_ = file.Close()
		if !keepTemporary {
			_ = os.Remove(temporary)
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, maxBytes+1))
	if err != nil {
		return File{}, fmt.Errorf("write attachment: %w", err)
	}
	if size > maxBytes {
		return File{}, apperr.New(http.StatusRequestEntityTooLarge, "ERR_UPLOAD_TOO_LARGE", "File exceeds the upload limit")
	}
	if err := file.Sync(); err != nil {
		return File{}, fmt.Errorf("sync attachment: %w", err)
	}
	if err := file.Close(); err != nil {
		return File{}, fmt.Errorf("close attachment: %w", err)
	}
	sniffed, err := sniffFile(temporary)
	if err != nil {
		return File{}, err
	}
	if options.Accept != "" && !accepts(sniffed, options.Accept) {
		return File{}, apperr.New(http.StatusUnsupportedMediaType, "ERR_ATTACHMENT_TYPE", "File type is not allowed")
	}
	thumbTemp := ""
	thumbName := idText + ".thumb.png"
	if safeImage(sniffed) {
		thumbTemp = filepath.Join(directory, ".thumb-"+idText)
		if err := makeThumbnail(temporary, thumbTemp, sniffed, maxMegapixels); err != nil {
			return File{}, err
		}
	}
	storedPath := filepath.ToSlash(filepath.Join("files", year, month, idText))
	finalPath := filepath.Join(directory, idText)
	thumbPath := filepath.Join(directory, thumbName)
	if err := os.Rename(temporary, finalPath); err != nil {
		if thumbTemp != "" {
			_ = os.Remove(thumbTemp)
		}
		return File{}, fmt.Errorf("install attachment: %w", err)
	}
	keepTemporary = true
	if thumbTemp != "" {
		if err := os.Rename(thumbTemp, thumbPath); err != nil {
			_ = os.Remove(finalPath)
			_ = os.Remove(thumbTemp)
			return File{}, fmt.Errorf("install attachment thumbnail: %w", err)
		}
	}
	cleanupInstalled := true
	defer func() {
		if cleanupInstalled {
			_ = os.Remove(finalPath)
			if thumbTemp != "" {
				_ = os.Remove(thumbPath)
			}
		}
	}()
	userUUID, ok := acl.ParseUUID(userID)
	if !ok {
		return File{}, acl.ErrValidation()
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return File{}, fmt.Errorf("begin attachment metadata transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		return File{}, fmt.Errorf("set attachment upload lock timeout: %w", err)
	}
	if _, err := acl.LockPath(ctx, tx, acl.Resource{Type: acl.Table, ID: tableUUID}); err != nil {
		return File{}, err
	}
	if err := acl.LockActiveUser(ctx, tx, userUUID); err != nil {
		return File{}, err
	}
	access, err := acl.AccessUncached(ctx, tx, userUUID, acl.Resource{Type: acl.Table, ID: tableUUID})
	if err != nil {
		return File{}, err
	}
	if !access.Visible {
		return File{}, acl.ErrNotFound()
	}
	if err := access.Check(acl.Write); err != nil {
		return File{}, err
	}
	queries := service.queries.WithTx(tx)
	currentOptions, err := queries.FieldForUpload(ctx, attachmentssql.FieldForUploadParams{ID: fieldUUID, TableID: tableUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, acl.ErrNotFound()
	}
	if err != nil {
		return File{}, fmt.Errorf("recheck attachment field: %w", err)
	}
	currentField, err := codec.Bind(codec.Attachment, currentOptions)
	if err != nil {
		return File{}, fmt.Errorf("rebind attachment field: %w", err)
	}
	var current codec.AttachmentOptions
	if err := json.Unmarshal(currentField.Options(), &current); err != nil {
		return File{}, fmt.Errorf("decode current attachment options: %w", err)
	}
	if current.Accept != "" && !accepts(sniffed, current.Accept) {
		return File{}, apperr.New(http.StatusUnsupportedMediaType, "ERR_ATTACHMENT_TYPE", "File type is not allowed")
	}
	_, err = queries.Insert(ctx, attachmentssql.InsertParams{
		ID: id, TableID: tableUUID, FieldID: fieldUUID, FileName: name, MimeType: sniffed,
		SizeBytes: size, Sha256: hash.Sum(nil), StoragePath: storedPath, HasThumb: thumbTemp != "", UploadedBy: userUUID,
	})
	if err != nil {
		return File{}, fmt.Errorf("insert attachment metadata: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		cleanupInstalled = false
		return File{}, fmt.Errorf("commit attachment metadata: %w", err)
	}
	cleanupInstalled = false
	fileDTO := File{ID: idText, Name: name, MIME: sniffed, Size: size}
	if thumbTemp != "" {
		thumbURL := codec.ThumbnailURL(idText)
		fileDTO.Thumb = &thumbURL
	}
	return fileDTO, nil
}

// Open resolves an attachment only after checking effective table read access.
func (service *Service) Open(ctx context.Context, userID, attachmentID string, thumbnail bool) (*OpenedFile, error) {
	id, ok := acl.ParseUUID(attachmentID)
	if !ok {
		return nil, acl.ErrNotFound()
	}
	metadata, err := service.queries.Get(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, acl.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("read attachment metadata: %w", err)
	}
	target := acl.Resource{Type: acl.Table, ID: metadata.TableID}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin attachment read: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		return nil, fmt.Errorf("set attachment read lock timeout: %w", err)
	}
	if _, err := acl.LockPath(ctx, tx, target); err != nil {
		return nil, err
	}
	userUUID, ok := acl.ParseUUID(userID)
	if !ok {
		return nil, acl.ErrValidation()
	}
	if err := acl.LockActiveUser(ctx, tx, userUUID); err != nil {
		return nil, err
	}
	access, err := acl.AccessUncached(ctx, tx, userUUID, target)
	if err != nil {
		return nil, err
	}
	if !access.Visible {
		return nil, acl.ErrNotFound()
	}
	if err := access.Check(acl.Read); err != nil {
		return nil, err
	}
	metadata, err = service.queries.WithTx(tx).Get(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || metadata.TableID != target.ID {
		return nil, acl.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("recheck attachment metadata: %w", err)
	}
	parts := strings.Split(metadata.StoragePath, "/")
	if len(parts) != 4 || parts[0] != "files" || len(parts[1]) != 4 || len(parts[2]) != 2 || parts[3] != attachmentID {
		return nil, errors.New("invalid attachment storage path")
	}
	directory, err := service.storageDirectory(parts[1], parts[2])
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, attachmentID)
	if thumbnail {
		if !metadata.HasThumb {
			return nil, acl.ErrNotFound()
		}
		path += ".thumb.png"
	}
	file, err := openRegular(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, acl.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("open attachment: %w", err)
	}
	name, err := cleanName(metadata.FileName)
	if err != nil {
		_ = file.Close()
		return nil, errors.New("invalid attachment name in metadata")
	}
	dto := File{ID: attachmentID, Name: name, MIME: metadata.MimeType, Size: metadata.SizeBytes}
	contentType, inline := metadata.MimeType, safeImage(metadata.MimeType)
	if thumbnail {
		contentType, inline = "image/png", true
		thumbURL := codec.ThumbnailURL(attachmentID)
		dto.Thumb = &thumbURL
	} else if metadata.HasThumb {
		thumbURL := codec.ThumbnailURL(attachmentID)
		dto.Thumb = &thumbURL
	}
	if err := tx.Commit(ctx); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("commit attachment read: %w", err)
	}
	return &OpenedFile{File: dto, Reader: file, ContentType: contentType, Inline: inline,
		SHA256: hex.EncodeToString(metadata.Sha256)}, nil
}

func (service *Service) storageDirectory(year, month string) (string, error) {
	if len(year) != 4 || len(month) != 2 {
		return "", errors.New("invalid attachment storage date")
	}
	if _, err := strconv.Atoi(year + month); err != nil {
		return "", errors.New("invalid attachment storage date")
	}
	current := service.root
	for _, segment := range []string{year, month} {
		current = filepath.Join(current, segment)
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create attachment directory: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("attachment path contains a non-directory or symlink")
		}
	}
	return current, nil
}

func newUUID() (pgtype.UUID, string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return pgtype.UUID{}, "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	id := pgtype.UUID{Bytes: value, Valid: true}
	return id, id.String(), nil
}

func cleanName(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", apperr.New(400, "ERR_VALIDATION", "Invalid file name")
	}
	if index := strings.LastIndexAny(value, `/\\`); index >= 0 {
		value = value[index+1:]
	}
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || utf8.RuneCountInString(value) > maxStoredFileName {
		return "", apperr.New(400, "ERR_VALIDATION", "Invalid file name")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", apperr.New(400, "ERR_VALIDATION", "Invalid file name")
		}
	}
	return value, nil
}

func sniffFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open attachment for MIME sniffing: %w", err)
	}
	defer file.Close()
	buffer := make([]byte, maxSniffBytes)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read attachment for MIME sniffing: %w", err)
	}
	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(buffer[:n]))
	if err != nil {
		return "", errors.New("MIME sniffing returned an invalid media type")
	}
	if n >= 12 && string(buffer[:4]) == "RIFF" && string(buffer[8:12]) == "WEBP" {
		mediaType = "image/webp"
	}
	return strings.ToLower(mediaType), nil
}

func accepts(mediaType, accepted string) bool {
	for _, candidate := range strings.Split(accepted, ", ") {
		if candidate == mediaType || strings.HasSuffix(candidate, "/*") && strings.HasPrefix(mediaType, strings.TrimSuffix(candidate, "*")) {
			return true
		}
	}
	return false
}

func safeImage(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func makeThumbnail(sourcePath, thumbnailPath, mediaType string, maxMegapixels int) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer file.Close()
	configuration, format, err := image.DecodeConfig(file)
	if err != nil {
		return apperr.New(400, "ERR_INVALID_IMAGE", "Image file cannot be decoded")
	}
	if configuration.Width <= 0 || configuration.Height <= 0 {
		return apperr.New(400, "ERR_INVALID_IMAGE", "Image dimensions are invalid")
	}
	w, h := uint64(configuration.Width), uint64(configuration.Height)
	if w*h > uint64(maxMegapixels)*1_000_000 {
		return apperr.New(413, "ERR_IMAGE_TOO_LARGE", "Image dimensions exceed the configured limit")
	}
	if !imageFormatMatches(mediaType, format) {
		return apperr.New(400, "ERR_INVALID_IMAGE", "Image format does not match detected type")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind image: %w", err)
	}
	decoded, _, err := image.Decode(file)
	if err != nil {
		return apperr.New(400, "ERR_INVALID_IMAGE", "Image file cannot be decoded")
	}
	width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	if width > thumbnailSize || height > thumbnailSize {
		ratio := min(float64(thumbnailSize)/float64(width), float64(thumbnailSize)/float64(height))
		width, height = max(1, int(float64(width)*ratio)), max(1, int(float64(height)*ratio))
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
	output, err := os.OpenFile(thumbnailPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create thumbnail: %w", err)
	}
	encodeErr := png.Encode(output, thumbnail)
	if encodeErr == nil {
		encodeErr = output.Sync()
	}
	closeErr := output.Close()
	if encodeErr != nil || closeErr != nil {
		_ = os.Remove(thumbnailPath)
		return errors.Join(encodeErr, closeErr)
	}
	return nil
}

func imageFormatMatches(mediaType, format string) bool {
	switch mediaType {
	case "image/png":
		return format == "png"
	case "image/jpeg":
		return format == "jpeg"
	case "image/gif":
		return format == "gif"
	case "image/webp":
		return format == "webp"
	default:
		return false
	}
}

func openRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("attachment path is not a regular file")
	}
	return os.Open(path)
}

// Delete removes an unreferenced attachment after serializing against row writes.
func (service *Service) Delete(ctx context.Context, userID, tableID, fieldID, attachmentID string) error {
	return service.delete(ctx, userID, tableID, fieldID, attachmentID, true)
}

// Discard cleans up a staged attachment, retaining it if any row still references it.
func (service *Service) Discard(ctx context.Context, userID, tableID, fieldID, attachmentID string) error {
	return service.delete(ctx, userID, tableID, fieldID, attachmentID, false)
}

func (service *Service) delete(ctx context.Context, userID, tableID, fieldID, attachmentID string,
	failIfReferenced bool) error {
	tableUUID, ok := acl.ParseUUID(tableID)
	if !ok {
		return acl.ErrNotFound()
	}
	fieldUUID, ok := acl.ParseUUID(fieldID)
	if !ok {
		return acl.ErrNotFound()
	}
	attachmentUUID, ok := acl.ParseUUID(attachmentID)
	if !ok {
		return acl.ErrNotFound()
	}
	target := acl.Resource{Type: acl.Table, ID: tableUUID}
	if err := service.acl.Require(ctx, userID, acl.Write, target); err != nil {
		return err
	}
	if _, err := service.queries.FieldForUpload(ctx, attachmentssql.FieldForUploadParams{ID: fieldUUID, TableID: tableUUID}); errors.Is(err, pgx.ErrNoRows) {
		return acl.ErrNotFound()
	} else if err != nil {
		return fmt.Errorf("read attachment field: %w", err)
	}
	metadata, err := service.queries.Get(ctx, attachmentUUID)
	if errors.Is(err, pgx.ErrNoRows) || metadata.TableID != tableUUID || metadata.FieldID != fieldUUID {
		return acl.ErrNotFound()
	}
	if err != nil {
		return fmt.Errorf("read attachment metadata: %w", err)
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin attachment deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		return fmt.Errorf("set attachment deletion lock timeout: %w", err)
	}
	if _, err := acl.LockPath(ctx, tx, target); err != nil {
		return err
	}
	userUUID, ok := acl.ParseUUID(userID)
	if !ok {
		return acl.ErrValidation()
	}
	if err := acl.LockActiveUser(ctx, tx, userUUID); err != nil {
		return err
	}
	access, err := acl.AccessUncached(ctx, tx, userUUID, target)
	if err != nil {
		return err
	}
	if !access.Visible {
		return acl.ErrNotFound()
	}
	if err := access.Check(acl.Write); err != nil {
		return err
	}
	queries := service.queries.WithTx(tx)
	fieldStorage, err := queries.FieldStorage(ctx, attachmentssql.FieldStorageParams{ID: fieldUUID, TableID: tableUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return acl.ErrNotFound()
	}
	if err != nil {
		return fmt.Errorf("read attachment storage field: %w", err)
	}
	if !fieldStorage.PgColumn.Valid || ddl.ValidateIdentifier(ddl.Book, fieldStorage.PgSchema) != nil ||
		ddl.ValidateIdentifier(ddl.Table, fieldStorage.PgName) != nil ||
		ddl.ValidateIdentifier(ddl.Column, fieldStorage.PgColumn.String) != nil {
		return errors.New("invalid attachment field storage identifiers")
	}
	physical := pgx.Identifier{fieldStorage.PgSchema, fieldStorage.PgName}.Sanitize()
	column := pgx.Identifier{fieldStorage.PgColumn.String}.Sanitize()
	query := "SELECT EXISTS (SELECT 1 FROM " + physical +
		" AS row_data CROSS JOIN LATERAL jsonb_array_elements(COALESCE(row_data." + column +
		", '[]'::jsonb)) AS item(value) WHERE item.value ->> 'id' = $1)"
	var referenced bool
	if err := tx.QueryRow(ctx, query, attachmentID).Scan(&referenced); err != nil {
		return fmt.Errorf("check attachment references: %w", err)
	}
	if referenced {
		if failIfReferenced {
			return apperr.New(409, "ERR_ATTACHMENT_IN_USE", "Attachment is still in use")
		}
		return nil
	}
	directoryParts := strings.Split(metadata.StoragePath, "/")
	if len(directoryParts) != 4 || directoryParts[0] != "files" || directoryParts[3] != attachmentID {
		return errors.New("invalid attachment storage path")
	}
	directory, err := service.storageDirectory(directoryParts[1], directoryParts[2])
	if err != nil {
		return err
	}
	originalPath := filepath.Join(directory, attachmentID)
	deletedPath := filepath.Join(directory, ".delete-"+attachmentID)
	thumbPath, deletedThumbPath := originalPath+".thumb.png", deletedPath+".thumb.png"
	if err := os.Rename(originalPath, deletedPath); err != nil {
		return fmt.Errorf("stage attachment deletion: %w", err)
	}
	if metadata.HasThumb {
		if err := os.Rename(thumbPath, deletedThumbPath); err != nil {
			_ = os.Rename(deletedPath, originalPath)
			return fmt.Errorf("stage thumbnail deletion: %w", err)
		}
	}
	restoreFiles := true
	defer func() {
		if restoreFiles {
			_ = os.Rename(deletedPath, originalPath)
			if metadata.HasThumb {
				_ = os.Rename(deletedThumbPath, thumbPath)
			}
		}
	}()
	if err := queries.Delete(ctx, attachmentssql.DeleteParams{ID: attachmentUUID, TableID: tableUUID, FieldID: fieldUUID}); err != nil {
		return fmt.Errorf("delete attachment metadata: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attachment deletion: %w", err)
	}
	restoreFiles = false
	_ = os.Remove(deletedPath)
	if metadata.HasThumb {
		_ = os.Remove(deletedThumbPath)
	}
	return nil
}
