package html2gemini

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

const destPath = "testdata"

// EnableExtraLogging turns on additional testing log output.
// Extra test logging can be enabled by setting the environment variable
// HTML2TEXT_EXTRA_LOGGING to "1" or "true".
var EnableExtraLogging bool

func init() {
	if v := os.Getenv("HTML2TEXT_EXTRA_LOGGING"); v == "1" || v == "true" {
		EnableExtraLogging = true
	}
}

// TODO Add tests for FromHTMLNode and FromReader.

func TestParseUTF8(t *testing.T) {
	htmlFiles := []struct {
		file                  string
		keywordShouldNotExist string
		keywordShouldExist    string
	}{
		{
			"utf8.html",
			"学习之道:美国公认学习第一书title",
			"次世界冠军赛上，我几近疯狂",
		},
		{
			"utf8_with_bom.xhtml",
			"1892年波兰文版序言title",
			"种新的波兰文本已成为必要",
		},
	}

	for _, htmlFile := range htmlFiles {
		bs, err := ioutil.ReadFile(path.Join(destPath, htmlFile.file))
		if err != nil {
			t.Fatal(err)
		}
		ctx := NewTraverseContext(Options{})
		text, err := FromReader(bytes.NewReader(bs), *ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, htmlFile.keywordShouldExist) {
			t.Fatalf("keyword %s should  exists in file %s", htmlFile.keywordShouldExist, htmlFile.file)
		}
		if strings.Contains(text, htmlFile.keywordShouldNotExist) {
			t.Fatalf("keyword %s should not exists in file %s", htmlFile.keywordShouldNotExist, htmlFile.file)
		}
	}
}

func TestStrippingWhitespace(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"test text",
			"test text",
		},
		{
			"  \ttext\ntext\n",
			"text text",
		},
		{
			"  \na \n\t \n \n a \t",
			"a a",
		},
		{
			"test        text",
			"test text",
		},
		{
			"test&nbsp;&nbsp;&nbsp; text&nbsp;",
			"test    text",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestParagraphsAndBreaks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"Test text",
			"Test text",
		},
		{
			"Test text<br>",
			"Test text",
		},
		{
			"Test text<br>Test",
			"Test text\nTest",
		},
		{
			"<p>Test text</p>",
			"Test text",
		},
		{
			"<p>Test text</p><p>Test text</p>",
			"Test text\n\nTest text",
		},
		{
			"\n<p>Test text</p>\n\n\n\t<p>Test text</p>\n",
			"Test text\n\nTest text",
		},
		{
			"\n<p>Test text<br/>Test text</p>\n",
			"Test text\nTest text",
		},
		{
			"\n<p>Test text<br> \tTest text<br></p>\n",
			"Test text\nTest text",
		},
		{
			"Test text<br><BR />Test text",
			"Test text\n\nTest text",
		},
		{
			"<pre>test1\ntest 2\n\ntest  3</pre>",
			"```\ntest1\ntest 2\n\ntest  3\n```",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestTables(t *testing.T) {
	testCases := []struct {
		input           string
		tabularOutput   string
		plaintextOutput string
	}{
		{
			"<table><tr><td></td><td></td></tr></table>",
			// Empty table
			// +--+--+
			// |  |  |
			// +--+--+
			"```\n+--+--+\n|  |  |\n+--+--+\n```",
			"",
		},
		{
			"<table><tr><td>cell1</td><td>cell2</td></tr></table>",
			// +-------+-------+
			// | cell1 | cell2 |
			// +-------+-------+
			"```\n+-------+-------+\n| cell1 | cell2 |\n+-------+-------+\n```",
			"cell1 cell2",
		},
		{
			"<table><tr><td>row1</td></tr><tr><td>row2</td></tr></table>",
			// +------+
			// | row1 |
			// | row2 |
			// +------+
			"```\n+------+\n| row1 |\n| row2 |\n+------+\n```",
			"row1 row2",
		},
		{
			`<table>
				<tbody>
					<tr><td><p>Row-1-Col-1-Msg123456789012345</p><p>Row-1-Col-1-Msg2</p></td><td>Row-1-Col-2</td></tr>
					<tr><td>Row-2-Col-1</td><td>Row-2-Col-2</td></tr>
				</tbody>
			</table>`,
			// +--------------------------------+-------------+
			// | Row-1-Col-1-Msg123456789012345 | Row-1-Col-2 |
			// | Row-1-Col-1-Msg2               |             |
			// | Row-2-Col-1                    | Row-2-Col-2 |
			// +--------------------------------+-------------+
			"```\n" + `+--------------------------------+-------------+
| Row-1-Col-1-Msg123456789012345 | Row-1-Col-2 |
| Row-1-Col-1-Msg2               |             |
| Row-2-Col-1                    | Row-2-Col-2 |
+--------------------------------+-------------+` + "\n```",
			`Row-1-Col-1-Msg123456789012345

Row-1-Col-1-Msg2

Row-1-Col-2 Row-2-Col-1 Row-2-Col-2` ,
		},
		{
			`<table>
			   <tr><td>cell1-1</td><td>cell1-2</td></tr>
			   <tr><td>cell2-1</td><td>cell2-2</td></tr>
			</table>`,
			// +---------+---------+
			// | cell1-1 | cell1-2 |
			// | cell2-1 | cell2-2 |
			// +---------+---------+
			"```\n+---------+---------+\n| cell1-1 | cell1-2 |\n| cell2-1 | cell2-2 |\n+---------+---------+\n```",
			"cell1-1 cell1-2 cell2-1 cell2-2",
		},
		{
			`<table>
				<thead>
					<tr><th>Header 1</th><th>Header 2</th></tr>
				</thead>
				<tfoot>
					<tr><td>Footer 1</td><td>Footer 2</td></tr>
				</tfoot>
				<tbody>
					<tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr>
					<tr><td>Row 2 Col 1</td><td>Row 2 Col 2</td></tr>
				</tbody>
			</table>`,
			"```\n" + `+-------------+-------------+
|  HEADER 1   |  HEADER 2   |
+-------------+-------------+
| Row 1 Col 1 | Row 1 Col 2 |
| Row 2 Col 1 | Row 2 Col 2 |
+-------------+-------------+
|  FOOTER 1   |  FOOTER 2   |
+-------------+-------------+` + "\n```",
			"Header 1 Header 2 Footer 1 Footer 2 Row 1 Col 1 Row 1 Col 2 Row 2 Col 1 Row 2 Col 2",
		},
		// Two tables in same HTML (goal is to test that context is
		// reinitialized correctly).
		{
			`<p>
				<table>
					<thead>
						<tr><th>Table 1 Header 1</th><th>Table 1 Header 2</th></tr>
					</thead>
					<tfoot>
						<tr><td>Table 1 Footer 1</td><td>Table 1 Footer 2</td></tr>
					</tfoot>
					<tbody>
						<tr><td>Table 1 Row 1 Col 1</td><td>Table 1 Row 1 Col 2</td></tr>
						<tr><td>Table 1 Row 2 Col 1</td><td>Table 1 Row 2 Col 2</td></tr>
					</tbody>
				</table>
				<table>
					<thead>
						<tr><th>Table 2 Header 1</th><th>Table 2 Header 2</th></tr>
					</thead>
					<tfoot>
						<tr><td>Table 2 Footer 1</td><td>Table 2 Footer 2</td></tr>
					</tfoot>
					<tbody>
						<tr><td>Table 2 Row 1 Col 1</td><td>Table 2 Row 1 Col 2</td></tr>
						<tr><td>Table 2 Row 2 Col 1</td><td>Table 2 Row 2 Col 2</td></tr>
					</tbody>
				</table>
			</p>`,
			"```\n" + `+---------------------+---------------------+
|  TABLE 1 HEADER 1   |  TABLE 1 HEADER 2   |
+---------------------+---------------------+
| Table 1 Row 1 Col 1 | Table 1 Row 1 Col 2 |
| Table 1 Row 2 Col 1 | Table 1 Row 2 Col 2 |
+---------------------+---------------------+
|  TABLE 1 FOOTER 1   |  TABLE 1 FOOTER 2   |
+---------------------+---------------------+
` + "```\n" + "\n```" + `
+---------------------+---------------------+
|  TABLE 2 HEADER 1   |  TABLE 2 HEADER 2   |
+---------------------+---------------------+
| Table 2 Row 1 Col 1 | Table 2 Row 1 Col 2 |
| Table 2 Row 2 Col 1 | Table 2 Row 2 Col 2 |
+---------------------+---------------------+
|  TABLE 2 FOOTER 1   |  TABLE 2 FOOTER 2   |
+---------------------+---------------------+` + "\n```",
			`Table 1 Header 1 Table 1 Header 2 Table 1 Footer 1 Table 1 Footer 2 Table 1 Row 1 Col 1 Table 1 Row 1 Col 2 Table 1 Row 2 Col 1 Table 1 Row 2 Col 2

Table 2 Header 1 Table 2 Header 2 Table 2 Footer 1 Table 2 Footer 2 Table 2 Row 1 Col 1 Table 2 Row 1 Col 2 Table 2 Row 2 Col 1 Table 2 Row 2 Col 2`,
		},
		{
			"_<table><tr><td>cell</td></tr></table>_",
			"_\n\n```\n+------+\n| cell |\n+------+\n```\n\n_",
			"_\n\ncell\n\n_",
		},
		{
			`<table>
				<tr>
					<th>Item</th>
					<th>Description</th>
					<th>Price</th>
				</tr>
				<tr>
					<td>Golang</td>
					<td>Open source programming language that makes it easy to build simple, reliable, and efficient software</td>
					<td>$10.99</td>
				</tr>
				<tr>
					<td>Hermes</td>
					<td>Programmatically create beautiful e-mails using Golang.</td>
					<td>$1.99</td>
				</tr>
			</table>`,
			"```\n" + `+--------+--------------------------------+--------+
|  ITEM  |          DESCRIPTION           | PRICE  |
+--------+--------------------------------+--------+
| Golang | Open source programming        | $10.99 |
|        | language that makes it easy    |        |
|        | to build simple, reliable, and |        |
|        | efficient software             |        |
| Hermes | Programmatically create        | $1.99  |
|        | beautiful e-mails using        |        |
|        | Golang.                        |        |
+--------+--------------------------------+--------+` + "\n```",
			"Item Description Price Golang Open source programming language that makes it easy to build simple, reliable, and efficient software $10.99 Hermes Programmatically create beautiful e-mails using Golang. $1.99",
		},
	}

	for _, testCase := range testCases {
		options := Options{
			PrettyTables:        true,
			PrettyTablesOptions: NewPrettyTablesOptions(),
		}
		// Check pretty tabular ASCII version.
		if msg, err := wantString(testCase.input, testCase.tabularOutput, options); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}

		// Check plain version.
		if msg, err := wantString(testCase.input, testCase.plaintextOutput); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestStrippingLists(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<ul></ul>",
			"",
		},
		{
			"<ul><li>item</li></ul>_",
			"* item\n\n_",
		},
		{
			"<li class='123'>item 1</li> <li>item 2</li>\n_",
			"* item 1\n* item 2\n_",
		},
		{
			"<li>item 1</li> \t\n <li>item 2</li> <li> item 3</li>\n_",
			"* item 1\n* item 2\n* item 3\n_",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}


func TestOmitLinks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<a></a>`,
			``,
		},
		{
			`<a href=""></a>`,
			``,
		},
		{
			`<a href="http://example.com/"></a>`,
			``,
		},
		{
			`<a href="">Link</a>`,
			`Link`,
		},
		{
			`<a href="http://example.com/">Link</a>`,
			`Link`,
		},
		{
			`<a href="http://example.com/"><span class="a">Link</span></a>`,
			`Link`,
		},
		{
			"<a href='http://example.com/'>\n\t<span class='a'>Link</span>\n\t</a>",
			`Link`,
		},
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"></a>`,
			`Example`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output, Options{OmitLinks: true}); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestLinkEscaping(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<a href="foo">display</a>`,
			"display\n\n=> foo  display",		//minor bug with extra space at present
		},
		{
			`<a href="foo spaced">display</a>`,
			"display\n\n=> foo%20spaced  display",		//minor bug with extra space at present
		},
		{
			`<a href="foo?bar+baz">display</a>`,
			"display\n\n=> foo?bar+baz  display",		//minor bug with extra space at present
		},
	}
	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestCitationStyleLinks(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<a></a>`,
			``,
		},
		{
			`<a href=""></a>`,
			``,
		},
		{
			`<a href="http://example.com/"></a>`,
			"[1]\n\n=> http://example.com/ [1]",
		},
		{
			`<a href="">Link</a>`,
			"Link",
		},
		{
			`<a href="http://example1.com/">Link1</a><a href="http://example2.com/">Link2</a>`,
			"Link1 [1] Link2 [2]\n\n=> http://example1.com/ [1] Link1\n=> http://example2.com/ [2] Link2",
		},
		{
			`<a href="http://example1.com/">Link1</a> (<a href="http://example2.com/">Link2</a>)`,
			"Link1 [1] (Link2 [2])\n\n=> http://example1.com/ [1] Link1\n=> http://example2.com/ [2] Link2",
		},
		{
			`<a href="http://example1.com/">Link1</a>? <a href="http://example2.com/">Link2</a>!`,
			"Link1 [1]? Link2 [2]!\n\n=> http://example1.com/ [1] Link1\n=> http://example2.com/ [2] Link2",
		},
		{
			`<a href="http://example1.com/">Link1</a><a href="http://example1.com/">Link1 again</a>`,
			"Link1 [1] Link1 again [2]\n\n=> http://example1.com/ [1] Link1\n=> http://example1.com/ [2] Link1 again",
		},
		{
			`<a href="http://example.com/"><span class="a">Link</span></a>`,
			"Link [1]\n\n=> http://example.com/ [1] Link",
		},
		{
			"<a href='http://example.com/'>\n\t<span class='a'>Link</span>\n\t</a>",
			"Link [1]\n\n=> http://example.com/ [1] Link",
		},
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"></a>`,
			"Example [1]\n\n=> http://example.com/ [1] Example",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestImageAltTags(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			`<img />`,
			``,
		},
		{
			`<img src="http://example.ru/hello.jpg" />`,
			``,
		},
		{
			`<img alt="Example"/>`,
			``,
		},
		{
			`<img src="http://example.ru/hello.jpg" alt="Example"/>`,
			``,
		},
		// Images do matter if they are in a link.
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"/></a>`,
			`Example [1]\n\n=> http://example.com/ [1] Example`,
		},
		{
			`<a href="http://example.com/"><img src="http://example.ru/hello.jpg" alt="Example"></a>`,
			`Example ( http://example.com/ )`,
		},
		{
			`<a href='http://example.com/'><img src='http://example.ru/hello.jpg' alt='Example'/></a>`,
			`Example ( http://example.com/ )`,
		},
		{
			`<a href='http://example.com/'><img src='http://example.ru/hello.jpg' alt='Example'></a>`,
			`Example ( http://example.com/ )`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestHeadings(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<h1>Test</h1>",
			"# Test",
		},
		{
			"\t<h1>\nTest</h1> ",
			"# Test",
		},
		{
			"\t<h1>\nTest line 1<br>Test 2</h1> ",
			"# Test line 1\nTest 2",
		},
		{
			"<h1>Test</h1> <h1>Test</h1>",
			"# Test\n\n# Test",
		},
		{
			"<h2>Test</h2>",
			"## Test",
		},
		{
			"<h1><a href='http://example.com/'>Test</a></h1>",
			"# Test [1]",
		},
		{
			"<h3> <span class='a'>Test </span></h3>",
			"### Test",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestBold(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<b>Test</b>",
			"*Test*",
		},
		{
			"\t<b>Test</b> ",
			"*Test*",
		},
		{
			"\t<b>Test line 1<br>Test 2</b> ",
			"*Test line 1\nTest 2*",
		},
		{
			"<b>Test</b> <b>Test</b>",
			"*Test* *Test*",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestDiv(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<div>Test</div>",
			"Test",
		},
		{
			"\t<div>Test</div> ",
			"Test",
		},
		{
			"<div>Test line 1<div>Test 2</div></div>",
			"Test line 1\nTest 2",
		},
		{
			"Test 1<div>Test 2</div> <div>Test 3</div>Test 4",
			"Test 1\nTest 2\nTest 3\nTest 4",
		},
		{
			"Test 1<div>&nbsp;Test 2&nbsp;</div>",
			"Test 1\nTest 2",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestBlockquotes(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<div>level 0<blockquote>level 1<br><blockquote>level 2</blockquote>level 1</blockquote><div>level 0</div></div>",
			"level 0\n> \n> level 1\n> \n>> level 2\n> \n> level 1\n\nlevel 0",
		},
		{
			"<blockquote>Test</blockquote>Test",
			"> \n> Test\n\nTest",
		},
		{
			"\t<blockquote> \nTest<br></blockquote> ",
			"> \n> Test\n>",
		},
		{
			"\t<blockquote> \nTest line 1<br>Test 2</blockquote> ",
			"> \n> Test line 1\n> Test 2",
		},
		{
			"<blockquote>Test</blockquote> <blockquote>Test</blockquote> Other Test",
			"> \n> Test\n\n> \n> Test\n\nOther Test",
		},
		{
			"<blockquote>Lorem ipsum Commodo id consectetur pariatur ea occaecat minim aliqua ad sit consequat quis ex commodo Duis incididunt eu mollit consectetur fugiat voluptate dolore in pariatur in commodo occaecat Ut occaecat velit esse labore aute quis commodo non sit dolore officia Excepteur cillum amet cupidatat culpa velit labore ullamco dolore mollit elit in aliqua dolor irure do</blockquote>",
			"> \n> Lorem ipsum Commodo id consectetur pariatur ea occaecat minim aliqua ad\n> sit consequat quis ex commodo Duis incididunt eu mollit consectetur fugiat\n> voluptate dolore in pariatur in commodo occaecat Ut occaecat velit esse\n> labore aute quis commodo non sit dolore officia Excepteur cillum amet\n> cupidatat culpa velit labore ullamco dolore mollit elit in aliqua dolor\n> irure do",
		},
		{
			"<blockquote>Lorem<b>ipsum</b><b>Commodo</b><b>id</b><b>consectetur</b><b>pariatur</b><b>ea</b><b>occaecat</b><b>minim</b><b>aliqua</b><b>ad</b><b>sit</b><b>consequat</b><b>quis</b><b>ex</b><b>commodo</b><b>Duis</b><b>incididunt</b><b>eu</b><b>mollit</b><b>consectetur</b><b>fugiat</b><b>voluptate</b><b>dolore</b><b>in</b><b>pariatur</b><b>in</b><b>commodo</b><b>occaecat</b><b>Ut</b><b>occaecat</b><b>velit</b><b>esse</b><b>labore</b><b>aute</b><b>quis</b><b>commodo</b><b>non</b><b>sit</b><b>dolore</b><b>officia</b><b>Excepteur</b><b>cillum</b><b>amet</b><b>cupidatat</b><b>culpa</b><b>velit</b><b>labore</b><b>ullamco</b><b>dolore</b><b>mollit</b><b>elit</b><b>in</b><b>aliqua</b><b>dolor</b><b>irure</b><b>do</b></blockquote>",
			"> \n> Lorem *ipsum* *Commodo* *id* *consectetur* *pariatur* *ea* *occaecat* *minim*\n> *aliqua* *ad* *sit* *consequat* *quis* *ex* *commodo* *Duis* *incididunt* *eu*\n> *mollit* *consectetur* *fugiat* *voluptate* *dolore* *in* *pariatur* *in* *commodo*\n> *occaecat* *Ut* *occaecat* *velit* *esse* *labore* *aute* *quis* *commodo*\n> *non* *sit* *dolore* *officia* *Excepteur* *cillum* *amet* *cupidatat* *culpa*\n> *velit* *labore* *ullamco* *dolore* *mollit* *elit* *in* *aliqua* *dolor* *irure*\n> *do*",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}

}

func TestIgnoreStylesScriptsHead(t *testing.T) {
	testCases := []struct {
		input  string
		output string
	}{
		{
			"<style>Test</style>",
			"",
		},
		{
			"<style type=\"text/css\">body { color: #fff; }</style>",
			"",
		},
		{
			"<link rel=\"stylesheet\" href=\"main.css\">",
			"",
		},
		{
			"<script>Test</script>",
			"",
		},
		{
			"<script src=\"main.js\"></script>",
			"",
		},
		{
			"<script type=\"text/javascript\" src=\"main.js\"></script>",
			"",
		},
		{
			"<script type=\"text/javascript\">Test</script>",
			"",
		},
		{
			"<script type=\"text/ng-template\" id=\"template.html\"><a href=\"http://google.com\">Google</a></script>",
			"",
		},
		{
			"<script type=\"bla-bla-bla\" id=\"template.html\">Test</script>",
			"",
		},
		{
			`<html><head><title>Title</title></head><body></body></html>`,
			"",
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantString(testCase.input, testCase.output); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestText(t *testing.T) {
	testCases := []struct {
		input string
		expr  string
	}{
		{
			`<li>
		  <a href="/new" data-ga-click="Header, create new repository, icon:repo"><span class="octicon octicon-repo"></span> New repository</a>
		</li>`,
			`\* New repository \( /new \)`,
		},
		{
			`hi

			<br>

	hello <a href="https://google.com">google</a>
	<br><br>
	test<p>List:</p>

	<ul>
		<li><a href="foo">Foo</a></li>
		<li><a href="http://www.microshwhat.com/bar/soapy">Barsoap</a></li>
        <li>Baz</li>
	</ul>
`,
			`hi
hello google[1]

test

List:

* Foo[2]
* Barsoap[3]
* Baz`,
		},
		// Malformed input html.
		{
			`hi

			hello <a href="https://google.com">google</a>

			test<p>List:</p>

			<ul>
				<li><a href="foo">Foo</a>
				<li><a href="/
		                bar/baz">Bar</a>
		        <li>Baz</li>
			</ul>
		`,
			`hi hello google[1] test

List:

* Foo[2]
* Bar[3]
* Baz`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantRegExp(testCase.input, testCase.expr); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

func TestPeriod(t *testing.T) {
	testCases := []struct {
		input string
		expr  string
	}{
		{
			`<p>Lorem ipsum <span>test</span>.</p>`,
			`Lorem ipsum test\.`,
		},
		{
			`<p>Lorem ipsum <span>test.</span></p>`,
			`Lorem ipsum test\.`,
		},
	}

	for _, testCase := range testCases {
		if msg, err := wantRegExp(testCase.input, testCase.expr); err != nil {
			t.Error(err)
		} else if len(msg) > 0 {
			t.Log(msg)
		}
	}
}

type StringMatcher interface {
	MatchString(string) bool
	String() string
}

type RegexpStringMatcher string

func (m RegexpStringMatcher) MatchString(str string) bool {
	return regexp.MustCompile(string(m)).MatchString(str)
}
func (m RegexpStringMatcher) String() string {
	return string(m)
}

type ExactStringMatcher string

func (m ExactStringMatcher) MatchString(str string) bool {
	return string(m) == str
}
func (m ExactStringMatcher) String() string {
	return string(m)
}

func wantRegExp(input string, outputRE string, options ...Options) (string, error) {
	return match(input, RegexpStringMatcher(outputRE), options...)
}

func wantString(input string, output string, options ...Options) (string, error) {
	return match(input, ExactStringMatcher(output), options...)
}

func match(input string, matcher StringMatcher, options ...Options) (string, error) {
	var ctxOptions Options
	if len(options) > 0 {
		ctxOptions = options[0]
	}
	ctx := NewTraverseContext(ctxOptions)
	text, err := FromString(input, *ctx)
	if err != nil {
		return "", err
	}
	if !matcher.MatchString(text) {
		return "", fmt.Errorf(`error: input did not match specified expression
Input:
>>>>
%v
<<<<

Output:
>>>>
%v
<<<<

Expected:
>>>>
%v
<<<<`,
			input,
			text,
			matcher.String(),
		)
	}

	var msg string

	if EnableExtraLogging {
		msg = fmt.Sprintf(
			`
input:

%v

output:

%v
`,
			input,
			text,
		)
	}
	return msg, nil
}

func Example() {
	inputHTML := `
<html>
	<head>
		<title>My Mega Service</title>
		<link rel=\"stylesheet\" href=\"main.css\">
		<style type=\"text/css\">body { color: #fff; }</style>
	</head>

	<body>
		<div class="logo">
			<a href="http://jaytaylor.com/"><img src="/logo-image.jpg" alt="Mega Service"/></a>
		</div>

		<h1>Welcome to your new account on my service!</h1>

		<p>
			Here is some more information:

			<ul>
				<li>Link 1: <a href="https://example.com">Example.com</a></li>
				<li>Link 2: <a href="https://example2.com">Example2.com</a></li>
				<li>Something else</li>
			</ul>
		</p>

		<table>
			<thead>
				<tr><th>Header 1</th><th>Header 2</th></tr>
			</thead>
			<tfoot>
				<tr><td>Footer 1</td><td>Footer 2</td></tr>
			</tfoot>
			<tbody>
				<tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr>
				<tr><td>Row 2 Col 1</td><td>Row 2 Col 2</td></tr>
			</tbody>
		</table>

<pre>
Preformatted content    with    spaces
    and indentation
</pre>
	</body>
</html>`

	ctx := NewTraverseContext(Options{PrettyTables: true, LinkEmitFrequency: 100})
	text, err := FromString(inputHTML, *ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(text)

	// Output:
	// Mega Service [1]
	//
	// # Welcome to your new account on my service!
	//
	// Here is some more information:
	//
	// * Link 1: Example.com [2]
	// * Link 2: Example2.com [3]
	// * Something else
	//
	// ```
    // +-------------+-------------+
	// |  HEADER 1   |  HEADER 2   |
	// +-------------+-------------+
	// | Row 1 Col 1 | Row 1 Col 2 |
	// | Row 2 Col 1 | Row 2 Col 2 |
	// +-------------+-------------+
	// |  FOOTER 1   |  FOOTER 2   |
	// +-------------+-------------+
    // ```
    //
    //```
    //Preformatted content    with    spaces
    //    and indentation
    //
    //```
    //
    // => http://jaytaylor.com/ [1] http://jaytaylor.com/
    // => https://example.com [2] https://example.com
    // => https://example2.com [3] https://example2.com
}
