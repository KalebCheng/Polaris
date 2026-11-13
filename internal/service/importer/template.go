package importer

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

const templateSheet = "users"
const guideSheet = "guide"

// BuildTemplate 生成导入模板（第一个工作表只放表头，第二个工作表放列说明与示例）。
// The data sheet must not contain a sample row: parsers treat every non-blank
// data-sheet row as a real import and would create a known sample account.
func BuildTemplate(lang string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", templateSheet); err != nil {
		return nil, err
	}
	if _, err := f.NewSheet(guideSheet); err != nil {
		return nil, err
	}
	for i, c := range columns {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		if err := f.SetCellStr(templateSheet, col+"1", c.columnHeader(lang)); err != nil {
			return nil, err
		}
		if err := f.SetCellStr(guideSheet, fmt.Sprintf("A%d", i+1), c.columnHeader(lang)); err != nil {
			return nil, err
		}
		if err := f.SetCellStr(guideSheet, fmt.Sprintf("B%d", i+1), c.Description); err != nil {
			return nil, err
		}
		if err := f.SetCellStr(guideSheet, fmt.Sprintf("C%d", i+1), c.Example); err != nil {
			return nil, err
		}
	}
	if err := f.SetColWidth(templateSheet, "A", "M", 22); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(guideSheet, "A", "A", 16); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(guideSheet, "B", "B", 90); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(guideSheet, "C", "C", 22); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c Column) columnHeader(lang string) string {
	if lang == "en" {
		return c.HeaderEN
	}
	return c.HeaderZH
}
