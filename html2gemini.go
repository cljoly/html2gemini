package html2gemini

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/olekukonko/tablewriter"
	"github.com/ssor/bom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Options provide toggles and overrides to control specific rendering behaviors.
type Options struct {
	PrettyTables                bool                 // Turns on pretty ASCII rendering for table elements.
	PrettyTablesOptions         *PrettyTablesOptions // Configures pretty ASCII rendering for table elements.
	OmitLinks                   bool                 // Turns on omitting links
	CitationStart               int                  //Start Citations from this number (default 1)
	CitationMarkers             bool                 //use footnote style citation markers
	LinkEmitFrequency           int                  //emit gathered links after approximately every n paras (otherwise when new heading, or blockquote)
	NumberedLinks               bool                 // number the links [1], [2] etc to match citation markers
	EmitImagesAsLinks           bool                 //emit referenced images as links e.g. <img src=href>
	ImageMarkerPrefix           string               //prefix when emitting images
	EmptyLinkPrefix             string               //prefix when emitting empty links (e.g. <a href=foo><img src=bar></a>
	ListItemToLinkWordThreshold int                  //max number of words in a list item having a single link that is converted to a plain gemini link
}

//NewOptions creates Options with default settings
func NewOptions() *Options {
	return &Options{
		PrettyTables:                false,
		PrettyTablesOptions:         NewPrettyTablesOptions(),
		OmitLinks:                   false,
		CitationStart:               1,
		CitationMarkers:             true,
		NumberedLinks:               true,
		LinkEmitFrequency:           2,
		EmitImagesAsLinks:           true,
		ImageMarkerPrefix:           "‡",
		EmptyLinkPrefix:             ">>",
		ListItemToLinkWordThreshold: 30,
	}
}

// PrettyTablesOptions overrides tablewriter behaviors
type PrettyTablesOptions struct {
	AutoFormatHeader     bool
	AutoWrapText         bool
	ReflowDuringAutoWrap bool
	ColWidth             int
	ColumnSeparator      string
	RowSeparator         string
	CenterSeparator      string
	HeaderAlignment      int
	FooterAlignment      int
	Alignment            int
	ColumnAlignment      []int
	NewLine              string
	HeaderLine           bool
	RowLine              bool
	AutoMergeCells       bool
	Borders              tablewriter.Border
}

// NewPrettyTablesOptions creates PrettyTablesOptions with default settings
func NewPrettyTablesOptions() *PrettyTablesOptions {
	return &PrettyTablesOptions{
		AutoFormatHeader:     true,
		AutoWrapText:         true,
		ReflowDuringAutoWrap: true,
		ColWidth:             tablewriter.MAX_ROW_WIDTH,
		ColumnSeparator:      tablewriter.COLUMN,
		RowSeparator:         tablewriter.ROW,
		CenterSeparator:      tablewriter.CENTER,
		HeaderAlignment:      tablewriter.ALIGN_DEFAULT,
		FooterAlignment:      tablewriter.ALIGN_DEFAULT,
		Alignment:            tablewriter.ALIGN_DEFAULT,
		ColumnAlignment:      []int{},
		NewLine:              tablewriter.NEWLINE,
		HeaderLine:           true,
		RowLine:              false,
		AutoMergeCells:       false,
		Borders:              tablewriter.Border{Left: true, Right: true, Bottom: true, Top: true},
	}
}

// FlushCitations emits a list of Gemini links gathered up to this point, if the para count exceeds the
// emit frequency
func (ctx *TextifyTraverseContext) CheckFlushCitations() {

	//	if ctx.linkAccumulator.emitParaCount > ctx.options.LinkEmitFrequency &&  ctx.citationCount > 0 {
	if ctx.linkAccumulator.emitParaCount > ctx.options.LinkEmitFrequency && len(ctx.linkAccumulator.linkArray) > (ctx.linkAccumulator.flushedToIndex+1) {
		ctx.FlushCitations()
	} else {
		ctx.linkAccumulator.emitParaCount += 1
	}
}

func (ctx *TextifyTraverseContext) FlushCitations() {
	ctx.emitGeminiCitations()
}

func (ctx *TextifyTraverseContext) ResetCitationCounters() {
	ctx.linkAccumulator.flushedToIndex = len(ctx.linkAccumulator.linkArray) - 1
	ctx.linkAccumulator.emitParaCount = 0
}

// FromHTMLNode renders text output from a pre-parsed HTML document.
func FromHTMLNode(doc *html.Node, ctx TextifyTraverseContext) (string, error) {

	if err := ctx.traverse(doc); err != nil {
		return "", err
	}
	//flush any remaining citations at the end
	ctx.forceFlushGeminiCitations()

	text := strings.TrimSpace(newlineRe.ReplaceAllString(
		strings.Replace(ctx.buf.String(), "\n ", "\n", -1), "\n\n"),
	)

	//somewhat hacky tidying up of start and end of blockquotes
	startQuote := regexp.MustCompile(`\n *\n+> \n`)
	text = startQuote.ReplaceAllString(text, "\n\n")
	endQuote := regexp.MustCompile(`\n> \n\n+`)
	text = endQuote.ReplaceAllString(text, "\n\n")
	text = endQuote.ReplaceAllString(text, "\n\n")

	return text, nil
}

// FromReader renders text output after parsing HTML for the specified
// io.Reader.
func FromReader(reader io.Reader, ctx TextifyTraverseContext) (string, error) {
	newReader, err := bom.NewReaderWithoutBom(reader)
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(newReader)
	if err != nil {
		return "", err
	}

	return FromHTMLNode(doc, ctx)
}

// FromString parses HTML from the input string, then renders the text form.
func FromString(input string, ctx TextifyTraverseContext) (string, error) {
	bs := bom.CleanBom([]byte(input))
	text, err := FromReader(bytes.NewReader(bs), ctx)
	if err != nil {
		return "", err
	}
	return text, nil
}

var (
	spacingRe = regexp.MustCompile(`[ \r\n\t]+`)
	newlineRe = regexp.MustCompile(`\n\n+`)
)

// traverseTableCtx holds text-related context.
type TextifyTraverseContext struct {
	buf bytes.Buffer

	prefix          string
	tableCtx        tableTraverseContext
	options         Options
	endsWithSpace   bool
	justClosedDiv   bool
	blockquoteLevel int
	lineLength      int
	isPre           bool
	linkAccumulator linkAccumulatorType
}

type linkAccumulatorType struct {
	emitParaCount  int
	linkArray      []citationLink
	flushedToIndex int
	tableNestLevel int
}

func newlinkAccumulator() *linkAccumulatorType {
	return &linkAccumulatorType{
		flushedToIndex: -1,
	}
}

type citationLink struct {
	index   int
	url     string
	display string
}

// tableTraverseContext holds table ASCII-form related context.
type tableTraverseContext struct {
	header     []string
	body       [][]string
	footer     []string
	tmpRow     int
	isInFooter bool
}

func (tableCtx *tableTraverseContext) init() {
	tableCtx.body = [][]string{}
	tableCtx.header = []string{}
	tableCtx.footer = []string{}
	tableCtx.isInFooter = false
	tableCtx.tmpRow = 0
}

func NewTraverseContext(options Options) *TextifyTraverseContext {

	//no options provided we need to set some default options for non-zero
	//types.

	//start links at 1, not 0 if not specified
	options.CitationStart = 1 //otherwise uses zero value which is 0

	var ctx = TextifyTraverseContext{
		buf:     bytes.Buffer{},
		options: options,
	}

	ctx.linkAccumulator = *newlinkAccumulator()

	return &ctx
}
func (ctx *TextifyTraverseContext) handleElement(node *html.Node) error {
	ctx.justClosedDiv = false

	prefix := ""

	switch node.DataAtom {
	case atom.Footer, atom.Nav:
		return nil

	case atom.Br:
		return ctx.emit("\n")

	case atom.H1, atom.H2, atom.H3:

		if node.DataAtom == atom.H1 {
			ctx.FlushCitations()
			prefix = "# "
		}
		if node.DataAtom == atom.H2 {
			ctx.FlushCitations()
			prefix = "## "
		}

		if node.DataAtom == atom.H3 {
			ctx.FlushCitations()
			prefix = "### "
		}

		ctx.emit("\n\n" + prefix)
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		return ctx.emit("\n\n")

	case atom.Blockquote:
		ctx.FlushCitations()
		//if err := ctx.emit("\n"); err != nil {
		//	return err
		//}
		ctx.blockquoteLevel++
		ctx.prefix = strings.Repeat(">", ctx.blockquoteLevel) + " "
		//if ctx.blockquoteLevel == 1 {
		//	if err := ctx.emit("\n"); err != nil {
		//		return err
		//	}
		//}
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		ctx.blockquoteLevel--
		ctx.prefix = strings.Repeat(">", ctx.blockquoteLevel)
		if ctx.blockquoteLevel > 0 {
			ctx.prefix += " "
		}
		//return ctx.emit("\n\n")
		return ctx.emit("")

	case atom.Div:

		if ctx.lineLength > 0 {
			if err := ctx.emit("\n"); err != nil {
				return err
			}
		}
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		var err error
		if !ctx.justClosedDiv {
			err = ctx.emit("\n")
		}
		ctx.justClosedDiv = true
		return err

	case atom.Li:

		//a test context to examine the list element to see if it just has a single link
		//in which case we'll output a link line, or no links in which case we output just a bullet
		testCtx := TextifyTraverseContext{}
		if err := testCtx.traverseChildren(node); err != nil {
			return err
		}

		//if content contains just one link, output a link instead of a bullet if within a specified number of
		//words
		maxSingletonLinkLength := ctx.options.ListItemToLinkWordThreshold
		if (len(strings.Split(testCtx.buf.String(), " ")) < maxSingletonLinkLength) && (len(testCtx.linkAccumulator.linkArray) == 1) {
			return ctx.emit("=> " + testCtx.linkAccumulator.linkArray[0].url + " " + testCtx.buf.String() + "\n")
		}

		//if no links, just emit a bullet with the text, ignoring any sub elements
		if len(testCtx.linkAccumulator.linkArray) == 0 {
			return ctx.emit("* " + testCtx.buf.String() + "\n")
		}

		//otherwise is mixed content, so keep traversing
		if err := ctx.emit("* "); err != nil {
			return err
		}

		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		return ctx.emit("\n")

	case atom.Img:
		//output images with a link to the image
		hrefLink := ""
		altText := ""
		if altText = getAttrVal(node, "alt"); altText != "" {
			altText = altText
		} else {
			if src := getAttrVal(node, "src"); src != "" {
				//try to ge the last element of the path
				fileName := filepath.Base(src)
				fileBase := strings.TrimSuffix(fileName, filepath.Ext(fileName))
				altText = fileBase
			}
		}
		altText = "[" + ctx.options.ImageMarkerPrefix + " " + altText + "]"
		altText = strings.ReplaceAll(altText, "_", " ")
		altText = strings.ReplaceAll(altText, "-", " ")
		altText = strings.ReplaceAll(altText, "  ", " ")

		if ctx.options.EmitImagesAsLinks {
			if err := ctx.emit(altText); err != nil {
				return err
			}

			if attrVal := getAttrVal(node, "src"); attrVal != "" {
				attrVal = ctx.normalizeHrefLink(attrVal)
				if !ctx.options.OmitLinks && attrVal != "" && altText != attrVal {
					hrefLink = ctx.addGeminiCitation(attrVal, altText)
				}
			}
			return ctx.emit(hrefLink)
		} else {
			return ctx.emit(altText)
		}

	case atom.A:
		linkText := ""
		// For simple link element content with single text node only, peek at the link text.
		if node.FirstChild != nil && node.FirstChild.NextSibling == nil && node.FirstChild.Type == html.TextNode {
			linkText = node.FirstChild.Data
		}

		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		// If image is the only child, the image will have been shown as a link with its alt text etc
		// so choose a simple marker for the link itself
		if img := node.FirstChild; img != nil && node.LastChild == img && img.DataAtom == atom.Img {
			linkText = ctx.options.EmptyLinkPrefix
			ctx.emit(" " + linkText)
		}

		hrefLink := ""
		if attrVal := getAttrVal(node, "href"); attrVal != "" {
			attrVal = ctx.normalizeHrefLink(attrVal)
			// Don't print link href if it matches link element content or if the link is empty.
			if !ctx.options.OmitLinks && attrVal != "" && linkText != attrVal {
				hrefLink = ctx.addGeminiCitation(attrVal, linkText)
			}
		}

		return ctx.emit(hrefLink)

	case atom.Ul:

		return ctx.paragraphHandler(node)

	case atom.P:

		//a test context to examine the list element to see if it just has a single link
		//in which case we'll output a link line, or no links in which case we output just a bullet
		testCtx := TextifyTraverseContext{}
		if err := testCtx.traverseChildren(node); err != nil {
			return err
		}

		//if content contains just one link, output a link instead of a para if within a specified number of
		//words
		maxSingletonLinkLength := ctx.options.ListItemToLinkWordThreshold
		if (len(strings.Split(testCtx.buf.String(), " ")) < maxSingletonLinkLength) && (len(testCtx.linkAccumulator.linkArray) == 1) {
			return ctx.emit("=> " + testCtx.linkAccumulator.linkArray[0].url + " " + testCtx.buf.String() + "\n")
		}

		//if no links, just emit a para with the text, ignoring any sub elements
		if len(testCtx.linkAccumulator.linkArray) == 0 {
			return ctx.emit(testCtx.buf.String() + "\n")
		}

		//else - mixed content
		return ctx.paragraphHandler(node)

	case atom.Table, atom.Tfoot, atom.Th, atom.Tr, atom.Td:

		if ctx.options.PrettyTables {
			return ctx.handleTableElement(node)
		} else if node.DataAtom == atom.Table {
			//just treat tables as a type of paragraph
			ctx.emit("\n\n⊞ table ⊞\n\n")
			return ctx.paragraphHandler(node)
		}

		if node.DataAtom == atom.Tr {
			//start a new line
			ctx.emit("\n")
		}

		return ctx.traverseChildren(node)

	case atom.Pre:
		ctx.emit("\n\n```\n")
		ctx.isPre = true
		err := ctx.traverseChildren(node)
		ctx.isPre = false
		ctx.emit("\n```\n\n")
		return err

	case atom.Style, atom.Script, atom.Head:
		// Ignore the subtree.
		return nil

	default:
		return ctx.traverseChildren(node)
	}
}

// paragraphHandler renders node children surrounded by double newlines.
func (ctx *TextifyTraverseContext) paragraphHandler(node *html.Node) error {
	ctx.CheckFlushCitations()

	if err := ctx.emit("\n\n"); err != nil {
		return err
	}

	if err := ctx.traverseChildren(node); err != nil {
		return err
	}
	if err := ctx.emit("\n\n"); err != nil {
		return err
	}

	return nil
}

// handleTableElement is only to be invoked when options.PrettyTables is active.
func (ctx *TextifyTraverseContext) handleTableElement(node *html.Node) error {
	if !ctx.options.PrettyTables {
		panic("handleTableElement invoked when PrettyTables not active")
	}

	switch node.DataAtom {
	case atom.Table:

		if ctx.linkAccumulator.tableNestLevel == 0 {
			if err := ctx.emit("\n\n```\n"); err != nil {
				return err
			}
		} else {
			if err := ctx.emit("\n\n"); err != nil {
				return err
			}
		}

		ctx.linkAccumulator.tableNestLevel++

		// Re-intialize all table context.
		ctx.tableCtx.init()

		// Browse children, enriching context with table data.
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		table := tablewriter.NewWriter(buf)
		if ctx.options.PrettyTablesOptions != nil {
			options := ctx.options.PrettyTablesOptions
			table.SetAutoFormatHeaders(options.AutoFormatHeader)
			table.SetAutoWrapText(options.AutoWrapText)
			table.SetReflowDuringAutoWrap(options.ReflowDuringAutoWrap)
			table.SetColWidth(options.ColWidth)
			table.SetColumnSeparator(options.ColumnSeparator)
			table.SetRowSeparator(options.RowSeparator)
			table.SetCenterSeparator(options.CenterSeparator)
			table.SetHeaderAlignment(options.HeaderAlignment)
			table.SetFooterAlignment(options.FooterAlignment)
			table.SetAlignment(options.Alignment)
			table.SetColumnAlignment(options.ColumnAlignment)
			table.SetNewLine(options.NewLine)
			table.SetHeaderLine(options.HeaderLine)
			table.SetRowLine(options.RowLine)
			table.SetAutoMergeCells(options.AutoMergeCells)
			table.SetBorders(options.Borders)
		}
		table.SetHeader(ctx.tableCtx.header)
		table.SetFooter(ctx.tableCtx.footer)
		table.AppendBulk(ctx.tableCtx.body)

		// Render the table using ASCII.
		table.Render()
		if err := ctx.emit(buf.String()); err != nil {
			return err
		}

		ctx.linkAccumulator.tableNestLevel--

		if ctx.linkAccumulator.tableNestLevel == 0 {
			return ctx.emit("```\n\n")
		} else {
			return ctx.emit("\n\n")
		}

	case atom.Tfoot:
		ctx.tableCtx.isInFooter = true
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		ctx.tableCtx.isInFooter = false

	case atom.Tr:
		ctx.tableCtx.body = append(ctx.tableCtx.body, []string{})
		if err := ctx.traverseChildren(node); err != nil {
			return err
		}
		ctx.tableCtx.tmpRow++

	case atom.Th:
		res, err := ctx.renderEachChild(node)
		if err != nil {
			return err
		}

		ctx.tableCtx.header = append(ctx.tableCtx.header, res)

	case atom.Td:
		res, err := ctx.renderEachChild(node)
		if err != nil {
			return err
		}

		if ctx.tableCtx.isInFooter {
			ctx.tableCtx.footer = append(ctx.tableCtx.footer, res)
		} else {
			ctx.tableCtx.body[ctx.tableCtx.tmpRow] = append(ctx.tableCtx.body[ctx.tableCtx.tmpRow], res)
		}

	}
	return nil
}

func (ctx *TextifyTraverseContext) traverse(node *html.Node) error {
	switch node.Type {
	default:
		return ctx.traverseChildren(node)

	case html.TextNode:
		var data string
		if ctx.isPre {
			data = node.Data
		} else {
			data = strings.TrimSpace(spacingRe.ReplaceAllString(node.Data, " "))
		}
		return ctx.emit(data)

	case html.ElementNode:
		return ctx.handleElement(node)
	}
}

func (ctx *TextifyTraverseContext) traverseChildren(node *html.Node) error {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if err := ctx.traverse(c); err != nil {
			return err
		}
	}

	return nil
}

// Tests r for being a character where no space should be inserted in front of.
func punctNoSpaceBefore(r rune) bool {
	switch r {
	case '.', ',', ';', '!', '?', ')', ']', '>':
		return true
	default:
		return false
	}
}

// Tests r for being a character where no space should be inserted after.
func punctNoSpaceAfter(r rune) bool {
	switch r {
	case '(', '[', '<':
		return true
	default:
		return false
	}
}
func (ctx *TextifyTraverseContext) emit(data string) error {
	if data == "" {
		return nil
	}

	var lines = []string{data}

	for _, line := range lines {
		runes := []rune(line)
		startsWithSpace := unicode.IsSpace(runes[0]) || punctNoSpaceBefore(runes[0])
		if !startsWithSpace && !ctx.endsWithSpace {
			if err := ctx.buf.WriteByte(' '); err != nil {
				return err
			}
			ctx.lineLength++
		}
		ctx.endsWithSpace = unicode.IsSpace(runes[len(runes)-1]) || punctNoSpaceAfter(runes[len(runes)-1])
		for _, c := range line {
			if _, err := ctx.buf.WriteString(string(c)); err != nil {
				return err
			}
			ctx.lineLength++
			if c == '\n' {
				ctx.lineLength = 0
				if ctx.prefix != "" {
					if _, err := ctx.buf.WriteString(ctx.prefix); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (ctx *TextifyTraverseContext) normalizeHrefLink(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimPrefix(link, "mailto:")
	return link
}

func formatGeminiCitation(idx int, showMarker bool) string {
	if showMarker {
		return fmt.Sprintf("[%d]", idx)
	} else {
		return ""
	}

}

func (ctx *TextifyTraverseContext) addGeminiCitation(url string, display string) string {

	if url[0:1] == "#" {
		//dont emit bookmarks to the same page (url starts #)
		return ""
	} else {
		citation := citationLink{
			index:   len(ctx.linkAccumulator.linkArray) + ctx.options.CitationStart,
			display: display,
			url:     url,
		}

		//spaces would mess up the gemini link, so check for them
		if strings.Contains(citation.url, " ") {
			//escape the spaces
			citation.url = strings.ReplaceAll(citation.url, " ", "%20")

		}
		ctx.linkAccumulator.linkArray = append(ctx.linkAccumulator.linkArray, citation)
		return formatGeminiCitation(citation.index, ctx.options.CitationMarkers)
	}

}

func (ctx *TextifyTraverseContext) forceFlushGeminiCitations() {
	// this method writes to the buffer directly instead of using `emit`, b/c we do not want to split long links

	if ctx.linkAccumulator.tableNestLevel > 0 {
		//dont emit citation list inside a table
		return
	}

	ctx.buf.WriteString("\n")

	//ctx.buf.WriteString("flushedtoindex: ")
	//ctx.buf.WriteString(formatGeminiCitation(ctx.linkAccumulator.flushedToIndex))
	ctx.buf.WriteByte('\n')

	for i, link := range ctx.linkAccumulator.linkArray {
		//	ctx.buf.WriteString(formatGeminiCitation(i))

		if i > ctx.linkAccumulator.flushedToIndex {
			ctx.buf.WriteString("=> ")
			ctx.buf.WriteString(link.url)
			ctx.buf.WriteByte(' ')
			ctx.buf.WriteString(formatGeminiCitation(link.index, ctx.options.NumberedLinks))
			ctx.buf.WriteByte(' ')
			ctx.buf.WriteString(link.display)
			ctx.buf.WriteByte('\n')
		}
	}

	ctx.buf.WriteByte('\n')

	ctx.ResetCitationCounters()

}
func (ctx *TextifyTraverseContext) emitGeminiCitations() {

	if len(ctx.linkAccumulator.linkArray) > ctx.linkAccumulator.flushedToIndex {
		//there are unflushed links
		ctx.forceFlushGeminiCitations()
	}
}

// renderEachChild visits each direct child of a node and collects the sequence of
// textuual representaitons separated by a single newline.
func (ctx *TextifyTraverseContext) renderEachChild(node *html.Node) (string, error) {
	buf := &bytes.Buffer{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		s, err := FromHTMLNode(c, *ctx)
		if err != nil {
			return "", err
		}
		if _, err = buf.WriteString(s); err != nil {
			return "", err
		}
		if c.NextSibling != nil {
			if err = buf.WriteByte('\n'); err != nil {
				return "", err
			}
		}
	}
	return buf.String(), nil
}

func getAttrVal(node *html.Node, attrName string) string {
	for _, attr := range node.Attr {
		if attr.Key == attrName {
			return attr.Val
		}
	}

	return ""
}
