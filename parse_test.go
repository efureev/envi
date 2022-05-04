package envi

import (
	"testing"
)

func Test_parseLine_basic(t *testing.T) {

	key := `TEST_KEY`
	value := `text`
	line := key + `=` + value

	k, v, c, err := parseLine(line)
	if err != nil {
		t.Fatalf("should be nil")
	}

	if k != key {
		t.Fatalf("`%s` should be `%s`", k, key)
	}
	if v != value {
		t.Fatalf("`%s` should be `%s`", v, value)
	}
	if c != `` {
		t.Fatalf("`%s` should be `%s`", v, value)
	}
}

func parseAndCompare(t *testing.T, rawLine, expectedKey, expectedValue, expectedComment string) {
	key, value, comment, _ := parseLine(rawLine)
	if key != expectedKey || value != expectedValue || comment != expectedComment {
		t.Errorf(
			"Expected '%v' to parse as '%v' => '%v' => '%v', got '%v' => '%v'  => '%v' instead",
			rawLine, expectedKey, expectedValue, expectedComment, key, value, comment)
	}
}

func TestParsing(t *testing.T) {
	// unquoted values
	parseAndCompare(t, "FOO=bar", "FOO", "bar", ``)

	// parse values with spaces around equal sign
	parseAndCompare(t, "FOO =bar", "FOO", "bar", ``)
	parseAndCompare(t, "FOO= bar", "FOO", "bar", ``)

	// parses double quoted values
	parseAndCompare(t, `FOO="bar"`, "FOO", "bar", ``)

	// parses single quoted values
	parseAndCompare(t, "FOO='bar'", "FOO", "bar", ``)

	// parses escaped double quotes
	parseAndCompare(t, `FOO="escaped\"bar"`, "FOO", `escaped"bar`, ``)

	// parses single quotes inside double quotes
	parseAndCompare(t, `FOO="'d'"`, "FOO", `'d'`, ``)

	// parses yaml style options
	parseAndCompare(t, "OPTION_A: 1", "OPTION_A", "1", ``)

	//parses yaml values with equal signs
	parseAndCompare(t, "OPTION_A: Foo=bar", "OPTION_A", "Foo=bar", ``)

	// parses non-yaml options with colons
	parseAndCompare(t, "OPTION_A=1:B", "OPTION_A", "1:B", ``)

	// parses export keyword
	parseAndCompare(t, "export OPTION_A=2", "OPTION_A", "2", ``)
	parseAndCompare(t, `export OPTION_B='\n'`, "OPTION_B", "\\n", ``)
	parseAndCompare(t, "export exportFoo=2", "exportFoo", "2", ``)
	parseAndCompare(t, "exportFOO=2", "exportFOO", "2", ``)
	parseAndCompare(t, "export_FOO =2", "export_FOO", "2", ``)
	parseAndCompare(t, "export.FOO= 2", "export.FOO", "2", ``)
	parseAndCompare(t, "export\tOPTION_A=2", "OPTION_A", "2", ``)
	parseAndCompare(t, "  export OPTION_A=2", "OPTION_A", "2", ``)
	parseAndCompare(t, "\texport OPTION_A=2", "OPTION_A", "2", ``)

	// it 'expands newlines in quoted strings' do
	// expect(env('FOO="bar\nbaz"')).to eql('FOO' => "bar\nbaz")
	parseAndCompare(t, `FOO="bar\nbaz"`, "FOO", "bar\nbaz", ``)

	// it 'parses variables with "." in the name' do
	// expect(env('FOO.BAR=foobar')).to eql('FOO.BAR' => 'foobar')
	parseAndCompare(t, "FOO.BAR=foobar", "FOO.BAR", "foobar", ``)

	// it 'parses variables with several "=" in the value' do
	// expect(env('FOO=foobar=')).to eql('FOO' => 'foobar=')
	parseAndCompare(t, "FOO=foobar=", "FOO", "foobar=", ``)

	// it 'strips unquoted values' do
	// expect(env('foo=bar ')).to eql('foo' => 'bar') # not 'bar '
	parseAndCompare(t, "FOO=bar ", "FOO", "bar", ``)

	// it 'ignores inline comments' do
	// expect(env("foo=bar # this is foo")).to eql('foo' => 'bar')
	parseAndCompare(t, "FOO=bar # this is foo", "FOO", "bar", `this is foo`)

	// it 'allows # in quoted value' do
	// expect(env('foo="bar#baz" # comment')).to eql('foo' => 'bar#baz')
	parseAndCompare(t, `FOO="bar#baz" # comment`, "FOO", "bar#baz", `comment`)

	parseAndCompare(t, "FOO='bar#baz' # comment", "FOO", "bar#baz", `comment`)
	parseAndCompare(t, `FOO="bar#baz#bang" # comment`, "FOO", "bar#baz#bang", `comment`)

	// it 'parses # in quoted values' do
	// expect(env('foo="ba#r"')).to eql('foo' => 'ba#r')
	// expect(env("foo='ba#r'")).to eql('foo' => 'ba#r')
	parseAndCompare(t, `FOO="ba#r"`, "FOO", "ba#r", ``)
	parseAndCompare(t, "FOO='ba#r'", "FOO", "ba#r", ``)

	//newlines and backslashes should be escaped
	parseAndCompare(t, `FOO="bar\n\ b\az"`, "FOO", "bar\n baz", ``)
	parseAndCompare(t, `FOO="bar\\\n\ b\az"`, "FOO", "bar\\\n baz", ``)

	parseAndCompare(t, `FOO="bar\\r\ b\az"`, "FOO", "bar\r baz", ``)

	parseAndCompare(t, `KEY="`, "KEY", "\"", ``)
	parseAndCompare(t, `KEY="value`, "KEY", "\"value", ``)

	// leading whitespace should be ignored
	parseAndCompare(t, " KEY =value", "KEY", "value", ``)
	parseAndCompare(t, "   KEY=value", "KEY", "value", ``)
	parseAndCompare(t, "\tKEY=value", "KEY", "value", ``)

}

func TestParsingMultilineComment(t *testing.T) {
	line := " ## # <-Comment-> \n KEY1=323"
	k, v, c, err := parseLine(line)
	if err != nil {
		t.Error("Should be nil")
	}

	if k != `KEY1` {
		t.Error("Should be `KEY1`")
	}
	if v != `323` {
		t.Errorf("Should be `323` instead  %v", v)
	}

	if c != `<-Comment->` {
		t.Errorf("Should be `<-Comment->` instaed %v", c)
	}
}

func TestParsingMultiline2Comment(t *testing.T) {
	line := " ## # <-Comment-> \n # Sub-comment \n app_session=Hi!"
	k, v, c, err := parseLine(line)
	if err != nil {
		t.Error("Should be nil")
	}

	if k != `app_session` {
		t.Error("Should be `app_session`")
	}
	if v != `Hi!` {
		t.Errorf("Should be `\"Hi!\"` instead  %v", v)
	}

	if c != "<-Comment->\nSub-comment" {
		t.Errorf("Should be `<-Comment->\nSub-comment` instaed %v", c)
	}
}

func TestParsingOnlyComment(t *testing.T) {
	line := " ## # <-Comment-> "
	k, v, c, err := parseLine(line)

	if err != ErrOnlyComment {
		t.Error("Should be ErrOnlyComment")
	}

	if k != `` {
		t.Error("Should be ``")
	}
	if v != `` {
		t.Error("Should be ``")
	}
	if c != `<-Comment->` {
		t.Error("Should be `<-Comment->`")
	}
}

func TestParsingWoKey(t *testing.T) {
	_, _, _, err := parseLine(`="value"`)
	if err == nil {
		t.Errorf("Expected \"%v\" to return error, but it didn't", err)
	}
}

func TestParsing_BadlyFormat(t *testing.T) {
	badlyFormattedLine := "lol$wut"
	_, _, _, err := parseLine(badlyFormattedLine)
	if err == nil {
		t.Errorf("Expected \"%v\" to return error, but it didn't", badlyFormattedLine)
	}
}
