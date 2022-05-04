package envi

import (
	"strings"
	"testing"
)

func TestNewRow(t *testing.T) {

	row := NewRow(`test-key`, `text`)

	if row.Value != `text` {
		t.Fatalf("should be `text`")
	}

	if row.Key != `TEST_KEY` {
		t.Fatalf("should be `TEST_KEY`")
	}

	if row.Comment != `` {
		t.Fatalf("should be ``")
	}

	row.SetComment(`Comment`)

	if row.Comment != `Comment` {
		t.Fatalf("should be `Comment`")
	}

}

func TestRow_Marshal(t *testing.T) {

	exp := `"text2"`
	row := NewRow(`test-key`, `text2`)
	str, err := row.Marshal()
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	if str != `TEST_KEY=`+exp {
		t.Fatalf("`%s` should be `TEST_KEY=%s`", str, exp)
	}

	exp = `"Hello tesxt!"`
	row = NewRow(`test-key`, `Hello tesxt!`)
	str, err = row.Marshal()
	if err != nil {
		t.Fatalf("should be `nil`")
	}

	if str != `TEST_KEY=`+exp {
		t.Fatalf("`%s` should be `TEST_KEY=%s`", str, exp)
	}

}

func TestRow_Unmarshal_1(t *testing.T) {
	exp := `"Hello tesxt!"`
	line := `TEST_KEY=` + exp
	r := &row{}
	err := r.Unmarshal(line)

	if err != nil || r.Key != `TEST_KEY` || r.Value != `Hello tesxt!` || r.Comment != `` {
		t.Fatal("Wrong!`")
	}
}

func TestRow_Unmarshal_MultiComment(t *testing.T) {
	line := []string{` #### <-- Comment --> ####`, `# sub-comment`, `TEST_KEY=432`}
	//line := []string{` #### --[ Comment ]-- ####`, `# sub-comment`, `TEST_KEY=432`}
	r := &row{}
	err := r.Unmarshal(strings.Join(line, "\n"))

	if err != nil || r.Key != `TEST_KEY` || r.Value != `432` || r.Comment != "<-- Comment -->\nsub-comment" {
		t.Fatal("Wrong!`")
	}
}

func TestNormalizeKey(t *testing.T) {

	list := map[string]string{
		`test-test`:                   `TEST_TEST`,
		`tes-2__-t-test`:              `TES_2_T_TEST`,
		`t---sa---=-ss-es-2__-t-test`: `T_SA_SS_ES_2_T_TEST`,
		`test.test`:                   `TEST.TEST`,
	}

	for in, exp := range list {
		if out := normalizeKey(in); out != exp {
			t.Fatalf(`Must be equal: '%s' == '%s'`, out, exp)
		}
	}
}

func TestNormalizeValue(t *testing.T) {

	list := map[string]string{
		``:                     ``,
		`1`:                    `1`,
		`0122`:                 `122`,
		`"012"`:                `"012"`,
		`'0122'`:               `"0122"`,
		`0.5`:                  `"0.5"`,
		`3m`:                   `"3m"`,
		`"rob,ken,robert1"`:    `"rob,ken,robert1"`,
		`'rob,ken,robert2'`:    `"rob,ken,robert2"`,
		`rob,ken,robert3`:      `"rob,ken,robert3"`,
		`red:1,green:2,blue:3`: `"red:1,green:2,blue:3"`,
		`true`:                 `true`,
		`false`:                `false`,
	}

	for in, exp := range list {
		if out := normalizeValue(in); out != exp {
			t.Fatalf(`Must be equal: [%s] == [%s]`, out, exp)
		}
	}
}
func TestMergeRowMap(t *testing.T) {

	rows1 := map[string]*row{
		`rob`:  NewRow(`rob`, `"robert3"`),
		`bool`: NewRow(`bool`, `true`),
	}

	rows2 := map[string]*row{
		`test`: NewRow(`test`, `text`),
		`bool`: NewRow(`bool`, `false`),
	}

	mergeRowMap(rows1, rows2)
}
