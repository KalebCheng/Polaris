package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Format 文件格式
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatCSV  Format = "csv"
)

var (
	// ErrUnsupportedFormat 扩展名不在支持范围内
	ErrUnsupportedFormat = errors.New("unsupported import file format")
	// ErrFileMismatch 扩展名与实际内容不符（如 .xlsx 实为纯文本）
	ErrFileMismatch = errors.New("file content does not match its extension")
	// ErrEmptyFile 没有任何数据行（只有表头或完全为空）
	ErrEmptyFile = errors.New("import file has no data rows")
)

// DetectFormat 依据扩展名识别格式，并用魔数确认内容（防止改名上传）。
func DetectFormat(fileName string, content []byte) (Format, error) {
	looksZip := bytes.HasPrefix(content, []byte("PK\x03\x04"))
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".xlsx":
		if !looksZip {
			return "", ErrFileMismatch
		}
		return FormatXLSX, nil
	case ".csv":
		if looksZip {
			return "", ErrFileMismatch
		}
		return FormatCSV, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

// ParseInput 解析入参
type ParseInput struct {
	FileName string
	Content  []byte
	// Lang 模板语言：zh | en（决定优先按哪一套表头逐字校验；另一套模板同样接受）
	Lang string
	// Delimiter 多值单元格分隔符（默认 ;）
	Delimiter string
	// MaxRows 数据行数上限（含空行；超过即拒绝，避免只解析前半段就报"超限"）
	MaxRows int
}

// ParseResult 解析结果
type ParseResult struct {
	Format Format
	// Rows 数据行；RowNumber 为文件行号（表头是第 1 行）
	Rows []Row
	// EmptyRows 完全空白的行数（跳过，不计入任何结果）
	EmptyRows int
}

// ErrTooManyRows 数据行数超过上限
type ErrTooManyRows struct{ Max int }

func (e ErrTooManyRows) Error() string {
	return fmt.Sprintf("import file exceeds %d rows", e.Max)
}

// Parse 读取文件并切分为结构化的行。表头严格校验：缺失列 / 未知列 / 顺序不符都整份拒绝。
// 返回的 err 为 *HeaderError 或 ErrTooManyRows 等，由调用方映射为业务错误。
func Parse(in ParseInput) (*ParseResult, error) {
	format, err := DetectFormat(in.FileName, in.Content)
	if err != nil {
		return nil, err
	}
	var raw [][]string
	if format == FormatXLSX {
		raw, err = readSheet(in.Content)
	} else {
		raw, err = readCSV(in.Content)
	}
	if err != nil {
		return nil, err
	}
	headerIdx := -1
	for i, cells := range raw {
		if !isBlank(cells) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil, ErrEmptyFile
	}
	if err := validateHeader(raw[headerIdx], expectedHeaders(in.Lang)); err != nil {
		return nil, err
	}
	out := &ParseResult{Format: format}
	delimiter := in.Delimiter
	if delimiter == "" {
		delimiter = ";"
	}
	for i := headerIdx + 1; i < len(raw); i++ {
		cells := raw[i]
		if isBlank(cells) {
			out.EmptyRows++
			continue
		}
		if in.MaxRows > 0 && len(out.Rows) >= in.MaxRows {
			return nil, ErrTooManyRows{Max: in.MaxRows}
		}
		out.Rows = append(out.Rows, rowFromCells(i+1, cells, delimiter))
	}
	if len(out.Rows) == 0 {
		return nil, ErrEmptyFile
	}
	return out, nil
}

// HeaderError 表头与模板不一致
type HeaderError struct {
	// Missing 缺失的表头（按模板顺序）
	Missing []string
	// Unexpected 多出的表头（按文件顺序）
	Unexpected []string
	// OutOfOrder 列齐全但顺序与模板不一致
	OutOfOrder bool
}

func (e *HeaderError) Error() string {
	return fmt.Sprintf("header mismatch: missing=%v unexpected=%v out_of_order=%v", e.Missing, e.Unexpected, e.OutOfOrder)
}

// expectedHeaders 返回某语言的候选表头集合：首选该语言模板，次选另一语言模板
// （管理员手上有中英任一份模板都可用，但列必须是模板里的列）。
func expectedHeaders(lang string) [][]string {
	first, second := "zh", "en"
	if lang == "en" {
		first, second = "en", "zh"
	}
	return [][]string{headerFor(first), headerFor(second)}
}

func headerFor(lang string) []string {
	out := make([]string, 0, len(columns))
	for _, c := range columns {
		if lang == "en" {
			out = append(out, c.HeaderEN)
		} else {
			out = append(out, c.HeaderZH)
		}
	}
	return out
}

// validateHeader 按候选表头集合逐一尝试（任一完全匹配即通过）。
func validateHeader(actual []string, candidates [][]string) error {
	var firstErr error
	for _, expected := range candidates {
		err := matchHeader(actual, expected)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// matchHeader 严格逐列比对；先用集合判定缺失/多出，再判定顺序，便于给出可读的详情。
func matchHeader(actual, expected []string) error {
	norm := func(s string) string { return strings.TrimSpace(strings.TrimPrefix(s, "\uFEFF")) }
	actualNames := make([]string, 0, len(actual))
	for _, cell := range actual {
		if name := norm(cell); name != "" {
			actualNames = append(actualNames, name)
		}
	}
	herr := &HeaderError{Missing: []string{}, Unexpected: []string{}}
	for _, name := range expected {
		if _, ok := indexOf(actualNames, name); !ok {
			herr.Missing = append(herr.Missing, name)
		}
	}
	for _, name := range actualNames {
		if _, ok := indexOf(expected, name); !ok {
			herr.Unexpected = append(herr.Unexpected, name)
		}
	}
	// 列齐全才有"顺序"可谈：按非空表头逐位比对模板顺序
	if len(herr.Missing) == 0 && len(herr.Unexpected) == 0 {
		for i, name := range expected {
			if i >= len(actualNames) || actualNames[i] != name {
				herr.OutOfOrder = true
				break
			}
		}
	}
	if len(herr.Missing) == 0 && len(herr.Unexpected) == 0 && !herr.OutOfOrder {
		return nil
	}
	return herr
}

func indexOf(items []string, target string) (int, bool) {
	for i, item := range items {
		if item == target {
			return i, true
		}
	}
	return -1, false
}

// readSheet 读取第一个工作表的全部单元格（稀疏行补齐为空串，保证列下标与表头一致）。
func readSheet(content []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, ErrFileMismatch
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrEmptyFile
	}
	raw, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read sheet: %w", err)
	}
	return raw, nil
}

func readCSV(content []byte) ([][]string, error) {
	content = bytes.TrimPrefix(content, []byte("\uFEFF"))
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("%w: csv must be UTF-8 encoded", ErrFileMismatch)
	}
	reader := csv.NewReader(bytes.NewReader(content))
	// 列数不固定（尾部空列可能被省略），按表头自行定宽
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	return records, nil
}

func isBlank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func rowFromCells(number int, cells []string, delimiter string) Row {
	at := func(key string) string {
		idx := indexOfColumns(key)
		if idx < 0 || idx >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[idx])
	}
	split := func(key string) []string {
		return splitMulti(at(key), delimiter)
	}
	return Row{
		Number:      number,
		Name:        at("name"),
		DisplayName: at("display_name"),
		Email:       at("email"),
		Phone:       at("phone"),
		CountryCode: at("country_code"),
		FirstName:   at("first_name"),
		LastName:    at("last_name"),
		Title:       at("title"),
		Department:  at("department"),
		JobLevel:    at("job_level"),
		JobTitles:   split("job_titles"),
		Roles:       split("roles"),
		Password:    at("password"),
	}
}

func indexOfColumns(key string) int {
	for i, c := range columns {
		if c.Key == key {
			return i
		}
	}
	return -1
}

func splitMulti(value, delimiter string) []string {
	if value == "" {
		return []string{}
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(value, delimiter) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
