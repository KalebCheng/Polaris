package importer

import (
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func headerLine(lang string) []string { return headerFor(lang) }

func buildXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, row := range rows {
		for j, cell := range row {
			col, err := excelize.ColumnNumberToName(j + 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.SetCellStr("Sheet1", col+strconv.Itoa(i+1), cell); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fullRow(overrides map[string]string) []string {
	row := make([]string, 0, len(columns))
	for _, c := range columns {
		v := c.Example
		if ov, ok := overrides[c.Key]; ok {
			v = ov
		}
		row = append(row, v)
	}
	return row
}

// 列清单是模板与解析的唯一来源：Row 结构必须逐字段对应一列。
func TestColumnsCoverRowFields(t *testing.T) {
	want := []string{"name", "display_name", "email", "phone", "country_code", "first_name", "last_name",
		"title", "department", "job_level", "job_titles", "roles", "password"}
	got := make([]string, 0, len(columns))
	for _, c := range columns {
		got = append(got, c.Key)
		if c.HeaderZH == "" || c.HeaderEN == "" || c.Description == "" {
			t.Fatalf("列 %s 缺少中英表头或说明", c.Key)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("列清单漂移：got=%v want=%v", got, want)
	}
}

func TestTemplateRoundTripsThroughParser(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		data, err := BuildTemplate(lang)
		if err != nil {
			t.Fatalf("生成模板失败: %v", err)
		}
		result, err := Parse(ParseInput{FileName: "tpl.xlsx", Content: data, Lang: lang, MaxRows: 10})
		if err != nil {
			t.Fatalf("模板应能回读（lang=%s）: %v", lang, err)
		}
		if len(result.Rows) != 1 || result.Rows[0].Name != "zhangsan" {
			t.Fatalf("模板示例行解析结果不符: %+v", result.Rows)
		}
	}
}

func TestParseXLSXMultiValueAndEmptyRows(t *testing.T) {
	rows := [][]string{
		headerLine("zh"),
		fullRow(map[string]string{"job_titles": "前端; 后端", "roles": "开发者;运维"}),
		{"", "", "", ""},
		fullRow(map[string]string{"name": "lisi"}),
	}
	data := buildXLSX(t, rows)
	result, err := Parse(ParseInput{FileName: "a.xlsx", Content: data, Lang: "zh", MaxRows: 10})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(result.Rows) != 2 || result.EmptyRows != 1 {
		t.Fatalf("行数不符: rows=%d empty=%d", len(result.Rows), result.EmptyRows)
	}
	r := result.Rows[0]
	if r.Number != 2 {
		t.Fatalf("行号应为 2，实际 %d", r.Number)
	}
	if strings.Join(r.JobTitles, "|") != "前端|后端" {
		t.Fatalf("职务多值拆分不符（全角分号不应拆分）: %v", r.JobTitles)
	}
	if strings.Join(r.Roles, "|") != "开发者|运维" {
		t.Fatalf("角色多值应按 ; 拆分: %v", r.Roles)
	}
	if result.Rows[1].Number != 4 {
		t.Fatalf("空行之后的真实行号应保留为 4，实际 %d", result.Rows[1].Number)
	}
}

func TestParseRejectsHeaderDrift(t *testing.T) {
	header := headerLine("zh")
	header[2] = "邮件" // 改了"邮箱"
	data := buildXLSX(t, [][]string{header, fullRow(nil)})
	_, err := Parse(ParseInput{FileName: "a.xlsx", Content: data, Lang: "zh", MaxRows: 10})
	var headerErr *HeaderError
	if !errors.As(err, &headerErr) {
		t.Fatalf("表头不一致应报 HeaderError，实际 %v", err)
	}
	if len(headerErr.Missing) != 1 || headerErr.Missing[0] != "邮箱" {
		t.Fatalf("应报告缺失列 邮箱: %+v", headerErr)
	}
	if len(headerErr.Unexpected) != 1 || headerErr.Unexpected[0] != "邮件" {
		t.Fatalf("应报告多出列 邮件: %+v", headerErr)
	}
}

func TestParseRejectsReorderedHeader(t *testing.T) {
	header := headerLine("en")
	header[0], header[1] = header[1], header[0]
	data := buildXLSX(t, [][]string{header, fullRow(nil)})
	_, err := Parse(ParseInput{FileName: "a.xlsx", Content: data, Lang: "en", MaxRows: 10})
	var headerErr *HeaderError
	if !errors.As(err, &headerErr) || !headerErr.OutOfOrder {
		t.Fatalf("顺序不符应判定 OutOfOrder: %v", err)
	}
}

func TestParseCSVAndLimits(t *testing.T) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	_ = writer.Write(headerLine("en"))
	_ = writer.Write(fullRow(nil))
	_ = writer.Write(fullRow(map[string]string{"name": "lisi"}))
	writer.Flush()
	_, err := Parse(ParseInput{FileName: "a.csv", Content: []byte(buf.String()), Lang: "en", MaxRows: 1})
	var tooMany ErrTooManyRows
	if !errors.As(err, &tooMany) {
		t.Fatalf("超过行数上限应报 ErrTooManyRows: %v", err)
	}
	if _, err := Parse(ParseInput{FileName: "a.csv", Content: []byte(buf.String()), Lang: "en", MaxRows: 10}); err != nil {
		t.Fatalf("csv 应解析成功: %v", err)
	}
}

func TestDetectFormatRejectsRenamedFiles(t *testing.T) {
	if _, err := DetectFormat("evil.xlsx", []byte("name,display_name\n")); !errors.Is(err, ErrFileMismatch) {
		t.Fatalf("纯文本改名为 .xlsx 应拒绝")
	}
	if _, err := DetectFormat("a.txt", []byte("x")); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("不支持的扩展名应拒绝")
	}
	if _, err := DetectFormat("a.csv", []byte("PK\x03\x04rest")); !errors.Is(err, ErrFileMismatch) {
		t.Fatalf("xlsx 改名为 .csv 应拒绝")
	}
}
