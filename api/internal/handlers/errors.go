package handlers

import (
	"errors"

	"github.com/mkutlak/alluredeck/api/internal/runner"
)

// Sentinel errors for HTTP request validation.
var (
	ErrResultsRequired       = errors.New("'results' array is required in the body")
	ErrResultsEmpty          = errors.New("'results' array is empty")
	ErrFileNameRequired      = errors.New("'file_name' attribute is required for all results")
	ErrDuplicateFileNames    = errors.New("duplicated file names in 'results'")
	ErrContentBase64Required = errors.New("'content_base64' attribute is required")
	ErrNoFilesProvided       = errors.New("no files provided in 'files[]' field")

	// tar.gz archive validation errors are owned by internal/runner so the
	// sync handler path and the staged worker share one set. Aliased here
	// for backward compatibility with existing tests and callers.
	ErrArchiveEmpty         = runner.ErrArchiveEmpty
	ErrArchiveTooManyFiles  = runner.ErrArchiveTooManyFiles
	ErrArchiveDecompBomb    = runner.ErrArchiveDecompBomb
	ErrArchiveNestedPath    = runner.ErrArchiveNestedPath
	ErrArchiveInvalidEntry  = runner.ErrArchiveInvalidEntry
	ErrArchiveDuplicateFile = runner.ErrArchiveDuplicateFile

	// report_id validation errors.
	ErrReportIDRequired = errors.New("report_id is required")
	ErrReportIDInvalid  = errors.New("report_id must be 'latest' or a positive integer")

	// ticket_url validation errors.
	ErrTicketURLInvalidScheme = errors.New("ticket_url must use http or https scheme")

	// errUnsupportedContentType is returned by parseResultsBody when the
	// Content-Type header is not application/json, multipart/form-data, or application/gzip.
	errUnsupportedContentType = errors.New("unsupported Content-Type")
)

// Package-level limits for tar.gz extraction (vars for testability).
// Tests temporarily override these to keep fixtures small. The handler path
// only consults them when the runtime config does not provide overrides, so
// flipping them here is sufficient to exercise the corresponding error
// branches without rebuilding huge archives.
//
//nolint:gochecknoglobals // overridden in tests to avoid creating huge archives
var (
	maxDecompressedBytes int64 = 1 << 30 // 1 GB
	maxArchiveFileCount        = 5000
)
