package envi

import (
	"strings"
	"testing"
)

func TestParseRows(t *testing.T) {
	lines := []string{
		`APP_SESSION=".example.com"`,
		`   `,
		`# Application URL`,
		`APP_URL="https://example.com"`,
		`APP_SECURE=true`,
	}

	parsedLines, err := ParseRows(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	if len(parsedLines) != 3 {
		t.Fatalf("should be `3`")
	}
}

func TestUnmarshal(t *testing.T) {
	lines := []string{
		`APP_SESSION=".example.com"`,
		`Z_ALPHA=zed`,
		`   `,
		`# Application URL`,
		`APP_URL="https://example.com"`,
		`APP_SECURE=true`,
		`Z_INDEX=true`,
		`SECURE_HTTP=true`,
		`APPLICATION="Hello!"`,
		`APP_SESSION="localhost"`,
		`APP_SESSION="127.0.0.1"`,
	}

	env, err := Unmarshal(strings.Join(lines, "\n"))
	if err != nil {
		t.Fatal("should be `nil`")
	}

	if len(env) != 4 {
		t.Fatal("should be `4`")
	}

	bCount, rCount := env.Counts()
	if bCount != 3 || env.BlocksCount() != 3 || rCount != 1 || env.RowsCount() != 1 {
		t.Fatal("should be `2` and `2`")
	}

	if env.Count() != 7 {
		t.Fatal("should be `7`")
	}

	res, err := env.Marshal()
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	exp := []string{
		"APP_SECURE=true",
		`# APP_SESSION="localhost"`,
		`# APP_SESSION="127.0.0.1"`,
		`APP_SESSION=".example.com"`,
		`# Application URL`,
		`APP_URL="https://example.com"`,
		``,
		`APPLICATION="Hello!"`,
		`SECURE_HTTP=true`,
		``,
		`Z_ALPHA="zed"`,
		`Z_INDEX=true`,
	}

	if res != strings.Join(exp, "\n") {
		t.Fatal("should be `equal`")
	}

	block := env.GetBlock(`APP`)
	if block.Count() != 3 {
		t.Fatal("should be `3`")
	}

	SetCommentTemplate(`# <-- `, ` -->`)
	block.
		SetComment(`Block for an Application Settings`)

	res, _ = env.Marshal()
	exp = []string{
		`# <-- Block for an Application Settings -->`,
		"APP_SECURE=true",
		`# APP_SESSION="localhost"`,
		`# APP_SESSION="127.0.0.1"`,
		`APP_SESSION=".example.com"`,
		`# Application URL`,
		`APP_URL="https://example.com"`,
		``,
		`APPLICATION="Hello!"`,
		`SECURE_HTTP=true`,
		``,
		`Z_ALPHA="zed"`,
		`Z_INDEX=true`,
	}

	if res != strings.Join(exp, "\n") {
		t.Fatal("should be `equal`")
	}

	row := env.Get(`APP_SESSION`)
	if row == nil || row.Value != ".example.com" {
		t.Fatal(`should be ".example.com"`)
	}
	row = env.Get(`z-alpha`)
	if row == nil || row.Value != "zed" {
		t.Fatal(`should be "zed"`)
	}

	row = env.Get(`Application`)
	if row == nil || row.Value != "Hello!" {
		t.Fatal(`should be "Hello!"`)
	}
}

func TestAddBlock(t *testing.T) {
	env := Env{}
	block := NewBlock(`app`).AddRow(`session`, `test`)
	env.Add(block)

	if len(env) != 1 || env.Count() != 1 || block.Count() != 1 {
		t.Fatal("should be `1`")
	}
	r := env.Get(`APP_SESSION`)
	if r == nil {
		t.Fatal("should not be `nil`")
	}

	env.Add(NewBlock(`app`).
		AddRow(`hash`, `hash`).
		AddRow(`session`, `test`))

	if len(env) != 1 || env.Count() != 2 || block.Count() != 2 {
		t.Fatal("should be `2`")
	}

	block = env.GetBlock(`app`)
	if block.Count() != 2 {
		t.Fatal("should be `2`")
	}

	r = block.GetRow(`hash`)
	if r == nil || r.Value != `hash` {
		t.Fatal("should be `hash`")
	}

	r = block.GetRow(`session`)
	if r == nil || r.Value != `test` {
		t.Fatal("should be `test`")
	}
}

func TestAddRow(t *testing.T) {
	env := Env{}
	env.Add(NewRow(`app-session`, `test`))

	if len(env) != 1 {
		t.Fatal("should be `1`")
	}

	r := env.Get(`APP_SESSION`)
	if r == nil {
		t.Fatal("should not be `nil`")
	}

	r2 := env[0]
	if r2 == nil {
		t.Fatal("should not be `nil`")
	}
	if r2.GetKey() != `APP_SESSION` {
		t.Fatal("should be `APP_SESSION`")
	}

	env.Add(NewRow(`app-hash`, `ha`))

	if len(env) != 1 {
		t.Fatal("should be `1`")
	}

	block := env.GetBlock(`app`)
	if block == nil {
		t.Fatal("should not be `nil`")
	}

	if block.Count() != 2 {
		t.Fatal("should be `2`")
	}

	env.Add(NewRow(`app-hash`, `te`))

	if len(env) != 1 || block.Count() != 2 {
		t.Fatal("should be `2`")
	}

	r = block.GetRow(`hash`)
	if r == nil || r.Value != `ha` {
		t.Fatal("should be `ha`")
	}
	env.MergeItems(NewRow(`app-hash`, `te2`))

	if r.Value != `te2` {
		t.Fatal("should be `te2`")
	}

	block.RemovePrefixedRow(`app-session`)

	if block.Count() != 1 {
		t.Fatal("should be 1")
	}

	r = block.GetPrefixedRow(`app-hash`)

	if r == nil {
		t.Fatal("should not be `nil`")
	}
	if r.Value != `te2` {
		t.Fatal("should be `te2`")
	}
}

func TestEnv_RemoveRow(t *testing.T) {
	env := Env{}
	env.Add(
		NewRow(`app-session`, `test`),
		NewRow(`app-hash`, `ha`),
		NewRow(`test`, `ha21`),
	)

	if env.Count() != 3 {
		t.Fatal("should be `3`")
	}

	env.RemoveRow(`test`)
	if env.Count() != 2 || env.Get(`test`) != nil {
		t.Fatal("should be `2`")
	}

	env.RemoveRow(`app-session`)
	if env.Count() != 1 || env.Get(`app-session`) != nil {
		t.Fatal("should be `1`")
	}

	env.RemoveRow(`app-hash`)
	if env.Count() != 0 || env.Get(`app-hash`) != nil {
		t.Fatal("should be `0`")
	}

	if env.Get(`app`) != nil {
		t.Fatal("should not be `nil`")
	}

	env.RemoveBlock(`app`)
	if env.Get(`app`) != nil {
		t.Fatal("should be `nil`")
	}
}

func TestSuspectBlock(t *testing.T) {
	blockPrefix, rowName := suspectBlock(`APP_SESSION`)

	if blockPrefix != `APP` || rowName != `SESSION` {
		t.Fatalf("should be `APP` & `SESSION`")
	}

	blockPrefix, rowName = suspectBlock(`SESSION`)

	if blockPrefix != `` || rowName != `SESSION` {
		t.Fatalf("should be `` & `SESSION`")
	}
}

func TestLoad(t *testing.T) {
	SetCommentTemplateByDefault()
	GroupRowsGreaterThen(1)
	env, err := Load(`stubs/.env`, `stubs/.env.local`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	if env.Count() != 11 {
		t.Fatalf("should be `%d`", env.Count())
	}

	if env.Get(`APP_URL`).Value != `http://example.dev` {
		t.Fatalf("should be `http://example.dev`")
	}

	if env.Get(`APP_ENV`).Value != `local` {
		t.Fatalf("should be `local`")
	}
	if env.Get(`APP_DEBUG`).Value != `true` {
		t.Fatalf("should be `true`")
	}

	err = env.Save(`stubs/.env.total`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
}

func TestParse(t *testing.T) {
	SetCommentTemplateByDefault()
	env, err := Load(`stubs/.env`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	appBlock := env.GetBlock(`app`)
	if appBlock.Comment != `Application section` {
		t.Fatal(`should be 'Application section'`)
	}
	nginxBlock := env.GetBlock(`CACHE`)
	if nginxBlock.Comment != `NGINX cache section` {
		t.Fatal(`should be 'NGINX cache section'`)
	}
	appName := appBlock.GetRow(`name`)
	if appName.Comment != `Application name` {
		t.Fatal(`should be 'Application name'`)
	}
}

func TestParseMultiCommentFile(t *testing.T) {
	SetCommentTemplate(`###  < `, `>  ###`)

	env, err := Load(`stubs/.env.multi-comment.example`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	resSlice := env.MarshalToSlice()

	if len(resSlice) != 10 {
		t.Fatalf("should be `10`, %d instead", len(resSlice))
	}

	if !strings.Contains(resSlice[2], `#`) ||
		strings.Contains(resSlice[2], "\n") ||
		!strings.Contains(resSlice[3], `#`) ||
		strings.Contains(resSlice[3], "\n") ||
		strings.Contains(resSlice[4], `#`) ||
		strings.Contains(resSlice[4], "\n") {
		t.Fatalf("wrong")
	}

	/*expLine := "###  < REDIS>  ###\nREDIS_CLIENT=\"predis\"\n# Redis standalong\nnot for docker: 127.0.0.1\nREDIS_HOST=\"127.0.0.1\"\n# REDIS_MODE=\"sentinel\"\n# REDIS_PASSWORD=\"null\"\n# Standalong port\nREDIS_PORT=6379\nREDIS_PREFIX=\"local\""
	exp := strings.Split(expLine, "\n")
	spew.Dump(exp)
	res, err := env.Marshal()
	spew.Dump(res, err)*/
}

/*
func TestParseShadowsFile(t *testing.T) {
	SetCommentTemplate(`###  < `, `>  ###`)

	env, err := Load(`stubs/.env.shadows.example`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	str := env.String()

	env2, err := Unmarshal(str)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	str2 := env2.String()

	if str != str2 {
		t.Fatalf("should be `equal`")
	}
}
*/
func TestParseFullCommentsFile(t *testing.T) {
	env, err := Load(`stubs/.env.all-commented.example`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	r1 := env.Get(`VENDOR_DATA_PATH`)
	if r1.Value != `/Volumes/Docker/data/ssp/vendor` || r1.Comment != `Path to vendor` || !r1.commented {
		t.Fatal(`wrong!`)
	}

	r2 := env.Get(`DB_DATA_PATH`)
	if r2.Value != `/Volumes/Docker/data/ssp/storage` || r2.Comment != "Path to db storage\nUse careful" || r2.commented {
		t.Fatal(`wrong!`)
	}

	SetMarshalingWithoutComments()
	sl := env.MarshalToSlice()
	if len(sl) != 2 || sl[0] != `DB_DATA_PATH="/Volumes/Docker/data/ssp/storage"` || sl[1] != `# VENDOR_DATA_PATH="/Volumes/Docker/data/ssp/vendor"` {
		t.Fatal(`Wrong`)
	}

	SetMarshalingWithoutCommentedRows()
	sl = env.MarshalToSlice()
	if len(sl) != 1 || sl[0] != `DB_DATA_PATH="/Volumes/Docker/data/ssp/storage"` {
		t.Fatal(`Wrong`)
	}
}

func TestParseFullExampleFile(t *testing.T) {
	SetCommentTemplate(`###  < `, `>  ###`)

	env, err := Load(`stubs/.env.example`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}
	SetMarshalingWithoutShadows()
	SetMarshalingWithoutCommentedRows()
	SetMarshalingWithoutComments()

	sl := env.MarshalToSlice()
	if len(sl) != 32 {
		t.Fatal(`wrong`)
	}

	env.Save(`stubs/.env.example.final`)
}

func TestParseTmpExampleFile(t *testing.T) {
	SetCommentTemplate(`###  < `, `>  ###`)

	env, err := Load(`stubs/.env.tmp`)
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	env.Save(`stubs/.env.example.final`)
}
