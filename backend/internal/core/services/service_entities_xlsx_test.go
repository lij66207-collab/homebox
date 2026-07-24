package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func buildXlsx(t *testing.T, rows [][]string) *excelize.File {
	t.Helper()

	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for i, row := range rows {
		for j, val := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, val))
		}
	}
	return f
}

func TestXlsxImport_CreatesItems(t *testing.T) {
	ctx := context.Background()

	dst, err := tRepos.Groups.GroupCreate(ctx, "xlsx-import-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	f := buildXlsx(t, [][]string{
		{"HB.import_ref", "HB.name", "HB.location", "HB.description", "HB.quantity"},
		{"xlsx-ref-1", "Xlsx Item One", "Kitchen", "from excel", "2"},
		{"xlsx-ref-2", "Xlsx Item Two", "Kitchen", "", "1"},
	})
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	imported, err := tSvc.Entities.XlsxImport(ctx, dst.ID, buf)
	require.NoError(t, err)
	assert.Equal(t, 2, imported)

	item, err := tRepos.Entities.GetByRef(ctx, dst.ID, "xlsx-ref-1")
	require.NoError(t, err)
	assert.Equal(t, "Xlsx Item One", item.Name)
	assert.Equal(t, "from excel", item.Description)
}

func TestXlsxImport_TrailingEmptyCellsPadded(t *testing.T) {
	ctx := context.Background()

	dst, err := tRepos.Groups.GroupCreate(ctx, "xlsx-pad-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	// Header has more columns than the data rows fill — excelize trims
	// trailing empty cells, the reader must pad them back out.
	f := buildXlsx(t, [][]string{
		{"HB.import_ref", "HB.name", "HB.location", "HB.description"},
		{"xlsx-pad-1", "Padded Item", "Garage", ""},
	})
	defer func() { _ = f.Close() }()

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	imported, err := tSvc.Entities.XlsxImport(ctx, dst.ID, buf)
	require.NoError(t, err)
	assert.Equal(t, 1, imported)
}

func TestXlsxImport_InvalidFileRejected(t *testing.T) {
	ctx := context.Background()

	dst, err := tRepos.Groups.GroupCreate(ctx, "xlsx-bad-"+fk.Str(4), uuid.Nil)
	require.NoError(t, err)

	_, err = tSvc.Entities.XlsxImport(ctx, dst.ID, strings.NewReader("this is not an xlsx file"))
	require.Error(t, err)
}
