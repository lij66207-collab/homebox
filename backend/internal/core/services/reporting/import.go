package reporting

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/xuri/excelize/v2"
)

// Required column headers in CSV/TSV imports. Exported so tests and other
// packages share the same canonical strings.
const (
	HeaderHBLocation = "HB.location"
	HeaderHBName     = "HB.name"
)

var (
	ErrNoHomeboxHeaders       = errors.New("no headers found")
	ErrMissingRequiredHeaders = errors.New("missing required headers `" + HeaderHBLocation + "` or `" + HeaderHBName + "`")
)

// determineSeparator determines the separator used in the CSV file
// It returns the separator as a rune and an error if it could not be determined
//
// It is assumed that the first row is the header row and that the separator is the same
// for all rows.
//
// Supported separators are `,` and `\t`
func determineSeparator(data []byte) (rune, error) {
	// First row
	firstRow := bytes.Split(data, []byte("\n"))[0]

	// find first comma or /t
	comma := bytes.IndexByte(firstRow, ',')
	tab := bytes.IndexByte(firstRow, '\t')

	switch {
	case comma == -1 && tab == -1:
		return 0, errors.New("could not determine separator")
	case tab > comma:
		return '\t', nil
	default:
		return ',', nil
	}
}

// separatorDetectionBufferSize is the buffer size for reading CSV headers
// to detect the separator (comma vs tab)
const separatorDetectionBufferSize = 4096

// readRawCsv reads a CSV file and returns the raw data as a 2D string array
// It determines the separator used in the CSV file and returns an error if
// it could not be determined
func readRawCsv(r io.Reader) ([][]string, error) {
	// Buffer for reading the first line to detect separator
	// We read up to 4KB which should be more than enough for any header row
	firstLineBuffer := make([]byte, separatorDetectionBufferSize)
	n, err := io.ReadFull(r, firstLineBuffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	firstLineBuffer = firstLineBuffer[:n]

	// Determine separator from first line
	sep, err := determineSeparator(firstLineBuffer)
	if err != nil {
		return nil, err
	}

	// Create a multi-reader combining what we already read and the rest
	combinedReader := io.MultiReader(bytes.NewReader(firstLineBuffer), r)
	reader := csv.NewReader(combinedReader)
	reader.Comma = sep

	return reader.ReadAll()
}

// readRawXlsx reads the first worksheet of an Excel (.xlsx) file and returns
// its rows in the same shape as readRawCsv (header row first). Rows shorter
// than the header row are padded with empty strings so the rectangular check
// downstream behaves the same as for CSV (excelize trims trailing empty
// cells, which CSV files express as trailing separators).
func readRawXlsx(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsx file: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("xlsx file contains no worksheets")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read xlsx worksheet: %w", err)
	}

	// Drop fully empty rows (e.g. trailing blank lines Excel likes to keep)
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		empty := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, row)
		}
	}

	if len(out) == 0 {
		return nil, errors.New("xlsx worksheet is empty")
	}

	// Pad short rows to the header width
	width := len(out[0])
	for i, row := range out {
		if len(row) < width {
			out[i] = append(row, make([]string, width-len(row))...)
		}
	}

	return out, nil
}

// parseHeaders parses the homebox headers from the CSV file and returns a map of the headers
// and their column index as well as a list of the field headers (HB.field.*) in the order
// they appear in the CSV file
//
// It returns an error if no homebox headers are found
func parseHeaders(headers []string) (hbHeaders map[string]int, fieldHeaders []string, err error) {
	hbHeaders = map[string]int{} // initialize map

	for col, h := range headers {
		if strings.HasPrefix(h, "HB.field.") {
			fieldHeaders = append(fieldHeaders, h)
		}

		if strings.HasPrefix(h, "HB.") {
			hbHeaders[h] = col
		}
	}

	required := []string{HeaderHBLocation, HeaderHBName}
	if !lo.EveryBy(required, func(h string) bool {
		return lo.HasKey(hbHeaders, h)
	}) {
		return nil, nil, ErrMissingRequiredHeaders
	}

	if len(hbHeaders) == 0 {
		return nil, nil, ErrNoHomeboxHeaders
	}

	return hbHeaders, fieldHeaders, nil
}
