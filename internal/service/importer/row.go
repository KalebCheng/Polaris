package importer

// Row 一行导入数据（已按列清单取值，多值列已拆分）。
type Row struct {
	// Number 文件行号（表头为第 1 行，数据从第 2 行开始）
	Number      int
	Name        string
	DisplayName string
	Email       string
	Phone       string
	CountryCode string
	FirstName   string
	LastName    string
	Title       string
	Department  string
	JobLevel    string
	JobTitles   []string
	Roles       []string
	// Password 初始密码（只在内存中流转；不得写入日志、响应或导入记录）
	Password string
}
