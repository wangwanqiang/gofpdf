/*
 * Copyright (c) 2013 Kurt Jung (Gmail: kurt.w.jung)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package gofpdf

// Version: 1.7
// Date:    2011-06-18
// Author:  Olivier PLATHEY
// Port to Go: Kurt Jung, 2013-07-15

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"path"
	"strings"
	"time"
)

type fmtBuffer struct {
	bytes.Buffer
}

func (b *fmtBuffer) printf(fmtStr string, args ...interface{}) {
	b.Buffer.WriteString(fmt.Sprintf(fmtStr, args...))
}

// New returns a pointer to a new Fpdf instance. Its methods are subsequently
// called to produce a single PDF document.
//
// orientationStr specifies the default page orientation. For portrait mode,
// specify "P" or "Portrait". For landscape mode, specify "L" or "Landscape".
// An empty string will be replaced with "P".
//
// unitStr specifies the unit of length used in size parameters for elements
// other than fonts, which are always measured in points. Specify "pt" for
// point, "mm" for millimeter, "cm" for centimeter, or "in" for inch. An empty
// string will be replaced with "mm".
//
// sizeStr specifies the page size. Acceptable values are "A3", "A4", "A5",
// "Letter", or "Legal". An empty string will be replaced with "A4".
//
// fontDirStr specifies the file system location in which font resources will
// be found. An empty string is replaced with ".".
func New(orientationStr, unitStr, sizeStr, fontDirStr string) (f *Fpdf) {
	f = new(Fpdf)
	if orientationStr == "" {
		orientationStr = "P"
	}
	if unitStr == "" {
		unitStr = "mm"
	}
	if sizeStr == "" {
		sizeStr = "A4"
	}
	if fontDirStr == "" {
		fontDirStr = "."
	}
	f.page = 0
	f.n = 2
	f.pages = make([]*bytes.Buffer, 0, 8)
	f.pages = append(f.pages, bytes.NewBufferString("")) // pages[0] is unused (1-based)
	f.pageSizes = make(map[int]sizeType)
	f.state = 0
	f.fonts = make(map[string]fontDefType)
	f.fontFiles = make(map[string]fontFileType)
	f.diffs = make([]string, 0, 8)
	f.images = make(map[string]imageInfoType)
	f.pageLinks = make([][]linkType, 0, 8)
	f.pageLinks = append(f.pageLinks, make([]linkType, 0, 0)) //pageLinks[0] is unused (1-based)
	f.links = make([]intLinkType, 0, 8)
	f.links = append(f.links, intLinkType{}) // links[0] is unused (1-based)
	f.inHeader = false
	f.inFooter = false
	f.lasth = 0
	f.fontFamily = ""
	f.fontStyle = ""
	f.fontSizePt = 12
	f.underline = false
	f.drawColor = "0 G"
	f.fillColor = "0 g"
	f.textColor = "0 g"
	f.colorFlag = false
	f.ws = 0
	f.fontpath = fontDirStr
	// Core fonts
	f.coreFonts = map[string]bool{
		"courier":      true,
		"helvetica":    true,
		"times":        true,
		"symbol":       true,
		"zapfdingbats": true,
	}
	// Scale factor
	switch unitStr {
	case "pt", "point":
		f.k = 1.0
	case "mm":
		f.k = 72.0 / 25.4
	case "cm":
		f.k = 72.0 / 2.54
	case "in", "inch":
		f.k = 72.0
	default:
		f.err = fmt.Errorf("Incorrect unit %s", unitStr)
		return
	}
	// Page sizes
	f.stdpageSizes = make(map[string]sizeType)
	f.stdpageSizes["a3"] = sizeType{841.89, 1190.55}
	f.stdpageSizes["a4"] = sizeType{595.28, 841.89}
	f.stdpageSizes["a5"] = sizeType{420.94, 595.28}
	f.stdpageSizes["letter"] = sizeType{612, 792}
	f.stdpageSizes["legal"] = sizeType{612, 1008}
	f.defPageSize = f.getpagesizestr(sizeStr)
	if f.err != nil {
		return
	}
	f.curPageSize = f.defPageSize
	// Page orientation
	orientationStr = strings.ToLower(orientationStr)
	switch orientationStr {
	case "p", "portrait":
		f.defOrientation = "P"
		f.w = f.defPageSize.wd
		f.h = f.defPageSize.ht
		// dbg("Assign h: %8.2f", f.h)
	case "l", "landscape":
		f.defOrientation = "L"
		f.w = f.defPageSize.ht
		f.h = f.defPageSize.wd
	default:
		f.err = fmt.Errorf("Incorrect orientation: %s", orientationStr)
		return
	}
	f.curOrientation = f.defOrientation
	f.wPt = f.w * f.k
	f.hPt = f.h * f.k
	// Page margins (1 cm)
	margin := 28.35 / f.k
	f.SetMargins(margin, margin, margin)
	// Interior cell margin (1 mm)
	f.cMargin = margin / 10
	// Line width (0.2 mm)
	f.lineWidth = 0.567 / f.k
	// 	Automatic page break
	f.SetAutoPageBreak(true, 2*margin)
	// Default display mode
	f.SetDisplayMode("default", "default")
	if f.err != nil {
		return
	}
	f.acceptPageBreak = func() bool {
		return f.autoPageBreak
	}
	// Enable compression
	f.SetCompression(true)
	// Set default PDF version number
	f.pdfVersion = "1.3"
	return
}

// Returns true if no processing errors have occurred.
func (f *Fpdf) Ok() bool {
	return f.err == nil
}

// Returns true if a processing error has occurred.
func (f *Fpdf) Err() bool {
	return f.err != nil
}

// Set the internal Fpdf error with formatted text to halt PDF generation; this
// may facilitate error handling by application.
//
// See the documentation for printing in the standard fmt package for details
// about fmtStr and args.
func (f *Fpdf) SetErrorf(fmtStr string, args ...interface{}) {
	if f.err == nil {
		f.err = fmt.Errorf(fmtStr, args...)
	}
}

// Summary of Fpdf instance that satisfies the fmt.Stringer interface.
func (f *Fpdf) String() string {
	return "Fpdf " + FPDF_VERSION
}

// Set error to halt PDF generation. This may facilitate error handling by
// application. See also Ok(), Err() and Error().
func (f *Fpdf) SetError(err error) {
	if f.err == nil && err != nil {
		f.err = err
	}
}

// Returns the internal Fpdf error; this will be nil if no error has occurred.
func (f *Fpdf) Error() error {
	return f.err
}

// Defines the left, top and right margins. By default, they equal 1 cm. Call
// this method to change them. If the value of the right margin is less than
// zero, it is set to the same as the left margin.
func (f *Fpdf) SetMargins(left, top, right float64) {
	f.lMargin = left
	f.tMargin = top
	if right < 0 {
		right = left
	}
	f.rMargin = right
}

// Defines the left margin. The method can be called before creating the first
// page. If the current abscissa gets out of page, it is brought back to the
// margin.
func (f *Fpdf) SetLeftMargin(margin float64) {
	f.lMargin = margin
	if f.page > 0 && f.x < margin {
		f.x = margin
	}
}

// Set the location in the file system of the font and font definition files.
func (f *Fpdf) SetFontLocation(fontDirStr string) {
	f.fontpath = fontDirStr
}

// Sets the function that lets the application render the page header. The
// specified function is automatically called by AddPage() and should not be
// called directly by the application. The implementation in Fpdf is empty, so
// you have to provide an appropriate function if you want page headers. fnc
// will typically be a closure that has access to the Fpdf instance and other
// document generation variables.
func (f *Fpdf) SetHeaderFunc(fnc func()) {
	f.headerFnc = fnc
}

// Sets the function that lets the application render the page footer. The
// specified function is automatically called by AddPage() and Close() and
// should not be called directly by the application. The implementation in Fpdf
// is empty, so you have to provide an appropriate function if you want page
// footers. fnc will typically be a closure that has access to the Fpdf
// instance and other document generation variables.
func (f *Fpdf) SetFooterFunc(fnc func()) {
	f.footerFnc = fnc
}

// Defines the top margin. The method can be called before creating the first
// page.
func (f *Fpdf) SetTopMargin(margin float64) {
	f.tMargin = margin
}

// Defines the right margin. The method can be called before creating the first
// page.
func (f *Fpdf) SetRightMargin(margin float64) {
	f.rMargin = margin
}

// Enables or disables the automatic page breaking mode. When enabling, the
// second parameter is the distance from the bottom of the page that defines
// the triggering limit. By default, the mode is on and the margin is 2 cm.
func (f *Fpdf) SetAutoPageBreak(auto bool, margin float64) {
	f.autoPageBreak = auto
	f.bMargin = margin
	f.pageBreakTrigger = f.h - margin
}

// Defines the way the document is to be displayed by the viewer. The zoom
// level can be set: pages can be displayed entirely on screen, occupy the full
// width of the window, use real size, be scaled by a specific zooming factor
// or use viewer default (configured in the Preferences menu of Adobe Reader).
// The page layout can be specified too: single at once, continuous display,
// two columns or viewer default.
//
// zoomStr can be "fullpage" to display the entire page on screen, "fullwidth"
// to use maximum width of window, "real" to use real size (equivalent to 100%
// zoom) or "default" to use viewer default mode.
//
// layoutStr can be "single" to display one page at once, "continuous" to
// display pages continuously, "two" to display two pages on two columns, or
// "default" to use viewer default mode.
func (f *Fpdf) SetDisplayMode(zoomStr, layoutStr string) {
	if f.err != nil {
		return
	}
	if layoutStr == "" {
		layoutStr = "default"
	}
	switch zoomStr {
	case "fullpage", "fullwidth", "real", "default":
		// || !is_string($zoom))
		f.zoomMode = zoomStr
	default:
		f.err = fmt.Errorf("Incorrect zoom display mode: %s", zoomStr)
		return
	}
	switch layoutStr {
	case "single", "continuous", "two", "default":
		f.layoutMode = layoutStr
	default:
		f.err = fmt.Errorf("Incorrect layout display mode: %s", layoutStr)
		return
	}
}

// Activates or deactivates page compression with zlib. When activated, the
// internal representation of each page is compressed, which leads to a
// compression ratio of about 2 for the resulting document. Compression is on
// by default.
func (f *Fpdf) SetCompression(compress bool) {
	// 	if(function_exists('gzcompress'))
	f.compress = compress
	// 	else
	// 		$this->compress = false;
}

// Defines the title of the document. isUTF8 indicates if the string is encoded
// in ISO-8859-1 (false) or UTF-8 (true).
func (f *Fpdf) SetTitle(titleStr string, isUTF8 bool) {
	if isUTF8 {
		titleStr = utf8toutf16(titleStr)
	}
	f.title = titleStr
}

// Defines the subject of the document. isUTF8 indicates if the string is encoded
// in ISO-8859-1 (false) or UTF-8 (true).
func (f *Fpdf) SetSubject(subjectStr string, isUTF8 bool) {
	if isUTF8 {
		subjectStr = utf8toutf16(subjectStr)
	}
	f.subject = subjectStr
}

// Defines the author of the document. isUTF8 indicates if the string is encoded
// in ISO-8859-1 (false) or UTF-8 (true).
func (f *Fpdf) SetAuthor(authorStr string, isUTF8 bool) {
	if isUTF8 {
		authorStr = utf8toutf16(authorStr)
	}
	f.author = authorStr
}

// Defines the keywords of the document. keywordStr is a space-delimited
// string, for example "invoice August". isUTF8 indicates if the string is
// encoded
func (f *Fpdf) SetKeywords(keywordsStr string, isUTF8 bool) {
	if isUTF8 {
		keywordsStr = utf8toutf16(keywordsStr)
	}
	f.keywords = keywordsStr
}

// Defines the creator of the document. isUTF8 indicates if the string is encoded
// in ISO-8859-1 (false) or UTF-8 (true).
func (f *Fpdf) SetCreator(creatorStr string, isUTF8 bool) {
	if isUTF8 {
		creatorStr = utf8toutf16(creatorStr)
	}
	f.creator = creatorStr
}

// Defines an alias for the total number of pages. It will be substituted as
// the document is closed.
func (f *Fpdf) AliasNbPages(aliasStr string) {
	if aliasStr == "" {
		aliasStr = "{nb}"
	}
	f.aliasNbPagesStr = aliasStr
}

// Begin document
func (f *Fpdf) open() {
	f.state = 1
}

// Terminates the PDF document. It is not necessary to call this method
// explicitly because Output() and OutputAndClose() do it automatically. If the
// document contains no page, AddPage() is called to prevent the generation of
// an invalid document.
func (f *Fpdf) Close() {
	if f.err != nil {
		return
	}
	if f.state == 3 {
		return
	}
	if f.page == 0 {
		f.AddPage()
		if f.err != nil {
			return
		}
	}
	// Page footer
	if f.footerFnc != nil {
		f.inFooter = true
		f.footerFnc()
		f.inFooter = false
	}
	// Close page
	f.endpage()
	// Close document
	f.enddoc()
	return
}

// Adds a new page with non-default orientation or size. See AddPage() for more
// details.
//
// See New() for a description of orientationStr.
//
// size specifies the size of the new page in the units established in New().
func (f *Fpdf) AddPageFormat(orientationStr string, size sizeType) {
	if f.err != nil {
		return
	}
	if f.state == 0 {
		f.open()
	}
	familyStr := f.fontFamily
	style := f.fontStyle
	if f.underline {
		style += "U"
	}
	fontsize := f.fontSizePt
	lw := f.lineWidth
	dc := f.drawColor
	fc := f.fillColor
	tc := f.textColor
	cf := f.colorFlag
	if f.page > 0 {
		// Page footer
		if f.footerFnc != nil {
			f.inFooter = true
			f.footerFnc()
			f.inFooter = false
		}
		// Close page
		f.endpage()
	}
	// Start new page
	f.beginpage(orientationStr, size)
	// 	Set line cap style to current value
	// f.out("2 J")
	f.outf("%d J", f.capStyle)
	// Set line width
	f.lineWidth = lw
	f.outf("%.2f w", lw*f.k)
	// 	Set font
	if familyStr != "" {
		f.SetFont(familyStr, style, fontsize)
		if f.err != nil {
			return
		}
	}
	// 	Set colors
	f.drawColor = dc
	if dc != "0 G" {
		f.out(dc)
	}
	f.fillColor = fc
	if fc != "0 g" {
		f.out(fc)
	}
	f.textColor = tc
	f.colorFlag = cf
	// 	Page header
	if f.headerFnc != nil {
		f.inHeader = true
		f.headerFnc()
		f.inHeader = false
	}
	// 	Restore line width
	if f.lineWidth != lw {
		f.lineWidth = lw
		f.outf("%.2f w", lw*f.k)
	}
	// Restore font
	if familyStr != "" {
		f.SetFont(familyStr, style, fontsize)
		if f.err != nil {
			return
		}
	}
	// Restore colors
	if f.drawColor != dc {
		f.drawColor = dc
		f.out(dc)
	}
	if f.fillColor != fc {
		f.fillColor = fc
		f.out(fc)
	}
	f.textColor = tc
	f.colorFlag = cf
	return
}

// Adds a new page to the document. If a page is already present, the Footer()
// method is called first to output the footer. Then the page is added, the
// current position set to the top-left corner according to the left and top
// margins, and Header() is called to display the header.
//
// The font which was set before calling is automatically restored. There is no
// need to call SetFont() again if you want to continue with the same font. The
// same is true for colors and line width.
//
// The origin of the coordinate system is at the top-left corner and increasing
// ordinates go downwards.
//
// See AddPageFormat() for a version of this method that allows the page size
// and orientation to be different than the default.
func (f *Fpdf) AddPage() {
	if f.err != nil {
		return
	}
	// dbg("AddPage")
	f.AddPageFormat(f.defOrientation, f.defPageSize)
	return
}

// Returns the current page number.
func (f *Fpdf) PageNo() int {
	return f.page
}

type clrType struct {
	r, g, b float64
}

func colorValue(r, g, b int) (clr clrType) {
	clr.r = float64(r) / 255.0
	clr.g = float64(g) / 255.0
	clr.b = float64(b) / 255.0
	return
}

func colorString(r, g, b int, grayStr, fullStr string) (str string) {
	clr := colorValue(r, g, b)
	if r == g && r == b {
		str = sprintf("%.3f %s", clr.r, grayStr)
	} else {
		str = sprintf("%.3f %.3f %.3f %s", clr.r, clr.g, clr.b, fullStr)
	}
	return
}

// Defines the color used for all drawing operations (lines, rectangles and
// cell borders). It is expressed in RGB components (0 - 255). The method can
// be called before the first page is created and the value is retained from
// page to page.
func (f *Fpdf) SetDrawColor(r, g, b int) {
	f.drawColor = colorString(r, g, b, "G", "RG")
	if f.page > 0 {
		f.out(f.drawColor)
	}
}

// Defines the color used for all filling operations (filled rectangles and
// cell backgrounds). It is expressed in RGB components (0 -255). The method
// can be called before the first page is created and the value is retained
// from page to page.
func (f *Fpdf) SetFillColor(r, g, b int) {
	f.fillColor = colorString(r, g, b, "g", "rg")
	f.colorFlag = f.fillColor != f.textColor
	if f.page > 0 {
		f.out(f.fillColor)
	}
}

// Defines the color used for text. It is expressed in RGB components (0 -
// 255). The method can be called before the first page is created and the
// value is retained from page to page.
func (f *Fpdf) SetTextColor(r, g, b int) {
	f.textColor = colorString(r, g, b, "g", "rg")
	f.colorFlag = f.fillColor != f.textColor
}

// Returns the length of a string in user units. A font must be selected.
func (f *Fpdf) GetStringWidth(s string) float64 {
	w := 0
	for _, ch := range s {
		w += f.currentFont.Cw[ch]
	}
	return float64(w) * f.fontSize / 1000
}

// Defines the line width. By default, the value equals 0.2 mm. The method can
// be called before the first page is created and the value is retained from
// page to page.
func (f *Fpdf) SetLineWidth(width float64) {
	f.lineWidth = width
	if f.page > 0 {
		f.outf("%.2f w", width*f.k)
	}
}

// Defines the line cap style. styleStr should be "butt", "round" or "square".
// A square style projects from the end of the line. The method can be called
// before the first page is created and the value is retained from page to
// page.
func (f *Fpdf) SetLineCapStyle(styleStr string) {
	var capStyle int
	switch styleStr {
	case "round":
		capStyle = 1
	case "square":
		capStyle = 2
	default:
		capStyle = 0
	}
	if capStyle != f.capStyle {
		f.capStyle = capStyle
		if f.page > 0 {
			f.outf("%d J", f.capStyle)
		}
	}
}

// Draws a line between points (x1, y1) and (x2, y2).
func (f *Fpdf) Line(x1, y1, x2, y2 float64) {
	f.outf("%.2f %.2f m %.2f %.2f l S", x1*f.k, (f.h-y1)*f.k, x2*f.k, (f.h-y2)*f.k)
}

func fillDrawOp(styleStr string) (opStr string) {
	switch strings.ToUpper(styleStr) {
	case "F":
		opStr = "f"
	case "FD", "DF":
		opStr = "B"
	default:
		opStr = "S"
	}
	return
}

// Outputs a rectangle. It can be drawn (border only), filled (with no border)
// or both. x and y specify the upper left corner of the rectangle. styleStr
// can be "F" for filled, "D" for outlined only, or "DF" or "FD" for outlined
// and filled. An empty string will be replaced with "D".
func (f *Fpdf) Rect(x, y, w, h float64, styleStr string) {
	f.outf("%.2f %.2f %.2f %.2f re %s", x*f.k, (f.h-y)*f.k, w*f.k, -h*f.k, fillDrawOp(styleStr))
}

// Draw a circle centered on point (x, y) with radius r. styleStr can be "F"
// for filled, "D" for outlined only, or "DF" or "FD" for outlined and filled.
// An empty string will be replaced with "D".
func (f *Fpdf) Circle(x, y, r float64, styleStr string) {
	f.Ellipse(x, y, r, r, 0, styleStr)
}

// Draw an ellipse centered at point (x, y). rx and ry specify its horizontal
// and vertical radii. degRotate specifies the counter-clockwise angle in
// degrees that the ellipse will be rotated. styleStr can be "F" for filled,
// "D" for outlined only, or "DF" or "FD" for outlined and filled. An empty
// string will be replaced with "D".
func (f *Fpdf) Ellipse(x, y, rx, ry, degRotate float64, styleStr string) {
	f.Arc(x, y, rx, ry, degRotate, 0, 360, styleStr)
}

// Outputs current point
func (f *Fpdf) point(x, y float64) {
	f.outf("%.2f %.2f m", x*f.k, (f.h-y)*f.k)
}

// Outputs quadratic curve from current point
func (f *Fpdf) curve(cx0, cy0, x1, y1, cx1, cy1 float64) {
	f.outf("%.2f %.2f %.2f %.2f %.2f %.2f c", cx0*f.k, (f.h-cy0)*f.k, x1*f.k,
		(f.h-y1)*f.k, cx1*f.k, (f.h-cy1)*f.k)
}

// Draws a single-segment quadratic Bézier curve. The curve starts at the
// point (x0, y0) and ends at the point (x1, y1). The control point (cx, cy)
// specifies the curvature. At the start point, the curve is tangent to the
// straight line between the start point and the control point. At the end
// point, the curve is tangent to the straight line between the end point and
// the control point. styleStr can be "F" for filled, "D" for outlined only, or
// "DF" or "FD" for outlined and filled. An empty string will be replaced with
// "D".
func (f *Fpdf) Curve(x0, y0, cx, cy, x1, y1 float64, styleStr string) {
	f.point(x0, y0)
	f.outf("%.2f %.2f %.2f %.2f v %s", cx*f.k, (f.h-cy)*f.k, x1*f.k, (f.h-y1)*f.k,
		fillDrawOp(styleStr))
}

// Draws a single-segment cubic Bézier curve. The curve starts at the point
// (x0, y0) and ends at the point (x1, y1). The control points (cx0, cy0) and
// (cx1, cy1) specify the curvature. At the start point, the curve is tangent
// to the straight line between the start point and the control point (cx0,
// cy0). At the end point, the curve is tangent to the straight line between
// the end point and the control point (cx1, cy1). styleStr can be "F" for
// filled, "D" for outlined only, or "DF" or "FD" for outlined and filled. An
// empty string will be replaced with "D".
func (f *Fpdf) CurveCubic(x0, y0, cx0, cy0, cx1, cy1, x1, y1 float64, styleStr string) {
	f.point(x0, y0)
	f.outf("%.2f %.2f %.2f %.2f %.2f %.2f c %s", cx0*f.k, (f.h-cy0)*f.k,
		x1*f.k, (f.h-y1)*f.k, cx1*f.k, (f.h-cy1)*f.k, fillDrawOp(styleStr))
}

// Draw an elliptical arc centered at point (x, y). rx and ry specify its
// horizontal and vertical radii. degRotate specifies the angle that the arc
// will be rotated. degStart and degEnd specify the starting and ending angle
// of the arc. All angles are specified in degrees and measured
// counter-clockwise from the 3 o'clock position. styleStr can be "F" for
// filled, "D" for outlined only, or "DF" or "FD" for outlined and filled. An
// empty string will be replaced with "D".
func (f *Fpdf) Arc(x, y, rx, ry, degRotate, degStart, degEnd float64, styleStr string) {
	x *= f.k
	y = (f.h - y) * f.k
	rx *= f.k
	ry *= f.k
	segments := int(degEnd-degStart) / 60
	if segments < 2 {
		segments = 2
	}
	angleStart := degStart * math.Pi / 180
	angleEnd := degEnd * math.Pi / 180
	angleTotal := angleEnd - angleStart
	dt := angleTotal / float64(segments)
	dtm := dt / 3
	if degRotate != 0 {
		a := -degRotate * math.Pi / 180
		f.outf("q %.2f %.2f %.2f %.2f %.2f %.2f cm", math.Cos(a), -1*math.Sin(a),
			math.Sin(a), math.Cos(a), x, y)
		x = 0
		y = 0
	}
	t := angleStart
	a0 := x + rx*math.Cos(t)
	b0 := y + ry*math.Sin(t)
	c0 := -rx * math.Sin(t)
	d0 := ry * math.Cos(t)
	f.point(a0/f.k, f.h-(b0/f.k))
	for j := 1; j <= segments; j++ {
		// Draw this bit of the total curve
		t = (float64(j) * dt) + angleStart
		a1 := x + rx*math.Cos(t)
		b1 := y + ry*math.Sin(t)
		c1 := -rx * math.Sin(t)
		d1 := ry * math.Cos(t)
		f.curve((a0+(c0*dtm))/f.k,
			f.h-((b0+(d0*dtm))/f.k),
			(a1-(c1*dtm))/f.k,
			f.h-((b1-(d1*dtm))/f.k),
			a1/f.k,
			f.h-(b1/f.k))
		a0 = a1
		b0 = b1
		c0 = c1
		d0 = d1
	}
	f.out(fillDrawOp(styleStr))
	if degRotate != 0 {
		f.out("Q")
	}
}

// Imports a TrueType, OpenType or Type1 font and makes it available. It is
// necessary to generate a font definition file first with the makefont
// utility. It is not necessary to call this function for the core PDF fonts
// (courier, helvetica, times, zapfdingbats).
//
// The JSON definition file (and the font file itself when embedding) must be
// present in the font directory. If it is not found, the error "Could not
// include font definition file" is set.
//
// family specifies the font family. The name can be chosen arbitrarily. If it
// is a standard family name, it will override the corresponding font. This
// string is used to subsequently set the font with the SetFont method.
//
// style specifies the font style. Acceptable values are (case insensitive) the
// empty string for regular style, "B" for bold, "I" for italic, or "BI" or
// "IB" for bold and italic combined.
//
// fileStr specifies the base name with ".json" extension of the font
// definition file to be added. The file will be loaded from the font directory
// specified in the call to New() or SetFontLocation().
func (f *Fpdf) AddFont(familyStr, styleStr, fileStr string) {
	if f.err != nil {
		return
	}
	// dbg("Adding family [%s], style [%s]", familyStr, styleStr)
	var ok bool
	familyStr = strings.ToLower(familyStr)
	if fileStr == "" {
		fileStr = strings.Replace(familyStr, " ", "", -1) + strings.ToLower(styleStr) + ".json"
	}
	fileStr = path.Join(f.fontpath, fileStr)
	styleStr = strings.ToUpper(styleStr)
	if styleStr == "IB" {
		styleStr = "BI"
	}
	fontkey := familyStr + styleStr
	// dbg("fontkey [%s]", fontkey)
	_, ok = f.fonts[fontkey]
	if ok {
		// dbg("fontkey found; returning")
		return
	}
	var info fontDefType
	info = f.loadfont(fileStr)
	if f.err != nil {
		return
	}
	info.I = len(f.fonts) + 1
	// dbg("font [%s], I [%d]", fileStr, info.I)
	if len(info.Diff) > 0 {
		// Search existing encodings
		n := -1
		for j, str := range f.diffs {
			if str == info.Diff {
				n = j
				break
			}
		}
		if n < 0 {
			n = len(f.diffs) + 1
			f.diffs[n] = info.Diff
		}
		info.DiffN = n
	}
	// dbg("font [%s], type [%s]", info.File, info.Tp)
	if len(info.File) > 0 {
		// Embedded font
		if info.Tp == "TrueType" {
			f.fontFiles[info.File] = fontFileType{length1: int64(info.OriginalSize)}
		} else {
			f.fontFiles[info.File] = fontFileType{length1: int64(info.Size1), length2: int64(info.Size2)}
		}
	}
	f.fonts[fontkey] = info
	return
}

// Sets the font used to print character strings. It is mandatory to call this
// method at least once before printing text or the resulting document will not
// be valid.
//
// The font can be either a standard one or a font added via the AddFont()
// method. Standard fonts use the Windows encoding cp1252 (Western Europe).
//
// The method can be called before the first page is created and the font is
// kept from page to page. If you just wish to change the current font size, it
// is simpler to call SetFontSize().
//
// Note: the font definition file must be accessible. An error is set if the
// file cannot be read.
//
// familyStr specifies the font fammily. It can be either a name defined by
// AddFont() or one of the standard families (case insensitive): "Courier" for
// fixed-width, "Helvetica" or "Arial" for sans serif, "Times" for serif,
// "Symbol" or "ZapfDingbats" for symbolic.
//
// styleStr can be "B" (bold), "I" (italic), "U" (underscore) or any
// combination. The default value (specified with an empty string) is regular.
// Bold and italic styles do not apply to Symbol and ZapfDingbats.
//
// size is the font size measured in points. The default value is the current
// size. If no size has been specified since the beginning of the document, the
// value taken is 12.
func (f *Fpdf) SetFont(familyStr, styleStr string, size float64) {
	// dbg("SetFont x %.2f, lMargin %.2f", f.x, f.lMargin)

	if f.err != nil {
		return
	}
	// dbg("SetFont")
	var ok bool
	if familyStr == "" {
		familyStr = f.fontFamily
	} else {
		familyStr = strings.ToLower(familyStr)
	}
	styleStr = strings.ToUpper(styleStr)
	f.underline = strings.Contains(styleStr, "U")
	if f.underline {
		styleStr = strings.Replace(styleStr, "U", "", -1)
	}
	if styleStr == "IB" {
		styleStr = "BI"
	}
	if size == 0.0 {
		size = f.fontSizePt
	}
	// Test if font is already selected
	if f.fontFamily == familyStr && f.fontStyle == styleStr && f.fontSizePt == size {
		return
	}
	// Test if font is already loaded
	fontkey := familyStr + styleStr
	_, ok = f.fonts[fontkey]
	if !ok {
		// Test if one of the core fonts
		if familyStr == "arial" {
			familyStr = "helvetica"
		}
		_, ok = f.coreFonts[familyStr]
		if ok {
			if familyStr == "symbol" || familyStr == "zapfdingbats" {
				styleStr = ""
			}
			fontkey = familyStr + styleStr
			_, ok = f.fonts[fontkey]
			if !ok {
				f.AddFont(familyStr, styleStr, "")
				if f.err != nil {
					return
				}
			}
		} else {
			f.err = fmt.Errorf("Undefined font: %s %s", familyStr, styleStr)
			return
		}
	}
	// Select it
	f.fontFamily = familyStr
	f.fontStyle = styleStr
	f.fontSizePt = size
	f.fontSize = size / f.k
	f.currentFont = f.fonts[fontkey]
	if f.page > 0 {
		f.outf("BT /F%d %.2f Tf ET", f.currentFont.I, f.fontSizePt)
	}
	return
}

// Defines the size of the current font in points.
func (f *Fpdf) SetFontSize(size float64) {
	if f.fontSizePt == size {
		return
	}
	f.fontSizePt = size
	f.fontSize = size / f.k
	if f.page > 0 {
		f.outf("BT /F%d %.2f Tf ET", f.currentFont.I, f.fontSizePt)
	}
}

// Creates a new internal link and returns its identifier. An internal link is
// a clickable area which directs to another place within the document. The
// identifier can then be passed to Cell(), Write(), Image() or Link(). The
// destination is defined with SetLink().
func (f *Fpdf) AddLink() int {
	f.links = append(f.links, intLinkType{})
	return len(f.links) - 1
}

// Defines the page and position a link points to. See AddLink().
func (f *Fpdf) SetLink(link int, y float64, page int) {
	if y == -1 {
		y = f.y
	}
	if page == -1 {
		page = f.page
	}
	f.links[link] = intLinkType{page, y}
}

// Add a new clickable link on current page
func (f *Fpdf) newLink(x, y, w, h float64, link int, linkStr string) {
	// linkList, ok := f.pageLinks[f.page]
	// if !ok {
	// linkList = make([]linkType, 0, 8)
	// f.pageLinks[f.page] = linkList
	// }
	f.pageLinks[f.page] = append(f.pageLinks[f.page],
		linkType{x * f.k, f.hPt - y*f.k, w * f.k, h * f.k, link, linkStr})
}

// Puts a link on a rectangular area of the page. Text or image links are
// generally put via Cell(), Write() or Image(), but this method can be useful
// for instance to define a clickable area inside an image. link is the value
// returned by AddLink().
func (f *Fpdf) Link(x, y, w, h float64, link int) {
	f.newLink(x, y, w, h, link, "")
}

// Puts a link on a rectangular area of the page. Text or image links are
// generally put via Cell(), Write() or Image(), but this method can be useful
// for instance to define a clickable area inside an image. linkStr is the
// target URL.
func (f *Fpdf) LinkString(x, y, w, h float64, linkStr string) {
	f.newLink(x, y, w, h, 0, linkStr)
}

// Prints a character string. The origin is on the left of the first character,
// on the baseline. This method allows to place a string precisely on the page,
// but it is usually easier to use Cell(), MultiCell() or Write() which are the
// standard methods to print text.
func (f *Fpdf) Text(x, y float64, txtStr string) {
	s := sprintf("BT %.2f %.2f Td (%s) Tj ET", x*f.k, (f.h-y)*f.k, f.escape(txtStr))
	if f.underline && txtStr != "" {
		s += " " + f.dounderline(x, y, txtStr)
	}
	if f.colorFlag {
		s = sprintf("q %s %s Q", f.textColor, s)
	}
	f.out(s)
}

// SetAcceptPageBreakFunc allows the application to control where page breaks
// occur.
//
// fnc is an application function (typically a closure) that is called by the
// library whenever a page break condition is met. The break is issued if true
// is returned. The default implementation returns a value according to the
// mode selected by SetAutoPageBreak. The function provided should not be
// called by the application.
//
// See tutorial 4 for an example of this function.
func (f *Fpdf) SetAcceptPageBreakFunc(fnc func() bool) {
	f.acceptPageBreak = fnc
}

// Prints a cell (rectangular area) with optional borders, background color and
// character string. The upper-left corner of the cell corresponds to the
// current position. The text can be aligned or centered. After the call, the
// current position moves to the right or to the next line. It is possible to
// put a link on the text.
//
// If automatic page breaking is enabled and the cell goes beyond the limit, a
// page break is done before outputting.
//
// w and h specify the width and height of the cell.
//
// txtStr specifies the text to display.
//
// borderStr specifies how the cell border will be drawn. An empty string
// indicates no border, "1" indicates a full border, and one or more of "L",
// "T", "R" and "B" indicate the left, top, right and bottom sides of the
// border.
//
// ln indicates where the current position should go after the call. Possible
// values are 0 (to the right), 1 (to the beginning of the next line), and 2
// (below). Putting 1 is equivalent to putting 0 and calling Ln() just after.
//
// alignStr allows the text to be aligned to the right ("R"), center ("C") or
// left ("L" or "").
//
// fill is true to paint the cell background or false to leave it transparent.
//
// link is the identifier returned by AddLink() or 0 for no internal link.
//
// linkStr is a target URL or empty for no external link. A non--zero value for
// link takes precedence over linkStr.
func (f *Fpdf) CellFormat(w, h float64, txtStr string, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	// dbg("CellFormat. h = %.2f, borderStr = %s", h, borderStr)
	if f.err != nil {
		return
	}
	borderStr = strings.ToUpper(borderStr)
	k := f.k
	if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
		// Automatic page break
		x := f.x
		ws := f.ws
		// dbg("auto page break, x %.2f, ws %.2f", x, ws)
		if ws > 0 {
			f.ws = 0
			f.out("0 Tw")
		}
		f.AddPageFormat(f.curOrientation, f.curPageSize)
		if f.err != nil {
			return
		}
		f.x = x
		if ws > 0 {
			f.ws = ws
			f.outf("%.3f Tw", ws*k)
		}
	}
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	var s fmtBuffer
	if fill || borderStr == "1" {
		var op string
		if fill {
			if borderStr == "1" {
				op = "B"
				// dbg("border is '1', fill")
			} else {
				op = "f"
				// dbg("border is empty, fill")
			}
		} else {
			// dbg("border is '1', no fill")
			op = "S"
		}
		/// dbg("(CellFormat) f.x %.2f f.k %.2f", f.x, f.k)
		s.printf("%.2f %.2f %.2f %.2f re %s ", f.x*k, (f.h-f.y)*k, w*k, -h*k, op)
	}
	if len(borderStr) > 0 && borderStr != "1" {
		// fmt.Printf("border is '%s', no fill\n", borderStr)
		x := f.x
		y := f.y
		left := x * k
		top := (f.h - y) * k
		right := (x + w) * k
		bottom := (f.h - (y + h)) * k
		if strings.Contains(borderStr, "L") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, top, left, bottom)
		}
		if strings.Contains(borderStr, "T") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, top, right, top)
		}
		if strings.Contains(borderStr, "R") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", right, top, right, bottom)
		}
		if strings.Contains(borderStr, "B") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, bottom, right, bottom)
		}
	}
	if len(txtStr) > 0 {
		var dx float64
		if alignStr == "R" {
			dx = w - f.cMargin - f.GetStringWidth(txtStr)
		} else if alignStr == "C" {
			dx = (w - f.GetStringWidth(txtStr)) / 2
			// dbg("center cell, dx %.2f\n", dx)
		} else {
			dx = f.cMargin
		}
		if f.colorFlag {
			s.printf("q %s ", f.textColor)
		}
		txt2 := strings.Replace(txtStr, "\\", "\\\\", -1)
		txt2 = strings.Replace(txt2, "(", "\\(", -1)
		txt2 = strings.Replace(txt2, ")", "\\)", -1)
		// if strings.Contains(txt2, "end of excerpt") {
		// dbg("f.h %.2f, f.y %.2f, h %.2f, f.fontSize %.2f, k %.2f", f.h, f.y, h, f.fontSize, k)
		// }
		s.printf("BT %.2f %.2f Td (%s) Tj ET", (f.x+dx)*k, (f.h-(f.y+.5*h+.3*f.fontSize))*k, txt2)
		//BT %.2F %.2F Td (%s) Tj ET',($this->x+$dx)*$k,($this->h-($this->y+.5*$h+.3*$this->FontSize))*$k,$txt2);
		if f.underline {
			s.printf(" %s", f.dounderline(f.x+dx, f.y+.5*h+.3*f.fontSize, txtStr))
		}
		if f.colorFlag {
			s.printf(" Q")
		}
		if link > 0 || len(linkStr) > 0 {
			f.newLink(f.x+dx, f.y+.5*h-.5*f.fontSize, f.GetStringWidth(txtStr), f.fontSize, link, linkStr)
		}
	}
	str := s.String()
	if len(str) > 0 {
		f.out(str)
	}
	f.lasth = h
	if ln > 0 {
		// Go to next line
		f.y += h
		if ln == 1 {
			f.x = f.lMargin
		}
	} else {
		f.x += w
	}
	return
}

// A simpler version of CellFormat with no fill, border, links or special
// alignment.
func (f *Fpdf) Cell(w, h float64, txtStr string) {
	f.CellFormat(w, h, txtStr, "", 0, "L", false, 0, "")
}

// A simpler printf-style version of CellFormat with no fill, border, links or
// special alignment. See documentation for the fmt package for details on
// fmtStr and args.
func (f *Fpdf) Cellf(w, h float64, fmtStr string, args ...interface{}) {
	f.CellFormat(w, h, sprintf(fmtStr, args...), "", 0, "L", false, 0, "")
}

// This method allows printing text with line breaks. They can be automatic (as
// soon as the text reaches the right border of the cell) or explicit (via the
// \n character). As many cells as necessary are output, one below the other.
//
// Text can be aligned, centered or justified. The cell block can be framed and
// the background painted. See CellFormat() for more details.
//
// w is the width of the cells. A value of zero indicates cells that reach to
// the right margin.
//
// h indicates the line height of each cell in the unit of measure specified in New().
func (f *Fpdf) MultiCell(w, h float64, txtStr, borderStr, alignStr string, fill bool) {
	// dbg("MultiCell")
	if alignStr == "" {
		alignStr = "J"
	}
	cw := &f.currentFont.Cw
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	wmax := (w - 2*f.cMargin) * 1000 / f.fontSize
	s := strings.Replace(txtStr, "\r", "", -1)
	nb := len(s)
	// if nb > 0 && s[nb-1:nb] == "\n" {
	if nb > 0 && []byte(s)[nb-1] == '\n' {
		nb--
		s = s[0:nb]
	}
	// dbg("[%s]\n", s)
	var b, b2 string
	b = "0"
	if len(borderStr) > 0 {
		if borderStr == "1" {
			borderStr = "LTRB"
			b = "LRT"
			b2 = "LR"
		} else {
			b2 = ""
			if strings.Contains(borderStr, "L") {
				b2 += "L"
			}
			if strings.Contains(borderStr, "R") {
				b2 += "R"
			}
			if strings.Contains(borderStr, "T") {
				b = b2 + "T"
			} else {
				b = b2
			}
		}
	}
	sep := -1
	i := 0
	j := 0
	l := 0.0
	ls := 0.0
	ns := 0
	nl := 1
	for i < nb {
		// Get next character
		c := []byte(s)[i]
		if c == '\n' {
			// Explicit line break
			if f.ws > 0 {
				f.ws = 0
				f.out("0 Tw")
			}
			f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
			i++
			sep = -1
			j = i
			l = 0
			ns = 0
			nl++
			if len(borderStr) > 0 && nl == 2 {
				b = b2
			}
			continue
		}
		if c == ' ' {
			sep = i
			ls = l
			ns++
		}
		l += float64(cw[c])
		if l > wmax {
			// Automatic line break
			if sep == -1 {
				if i == j {
					i++
				}
				if f.ws > 0 {
					f.ws = 0
					f.out("0 Tw")
				}
				f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
			} else {
				if alignStr == "J" {
					if ns > 1 {
						f.ws = (wmax - ls) / 1000 * f.fontSize / float64(ns-1)
					} else {
						f.ws = 0
					}
					f.outf("%.3f Tw", f.ws*f.k)
				}
				f.CellFormat(w, h, s[j:sep], b, 2, alignStr, fill, 0, "")
				i = sep + 1
			}
			sep = -1
			j = i
			l = 0
			ns = 0
			nl++
			if len(borderStr) > 0 && nl == 2 {
				b = b2
			}
		} else {
			i++
		}
	}
	// Last chunk
	if f.ws > 0 {
		f.ws = 0
		f.out("0 Tw")
	}
	if len(borderStr) > 0 && strings.Contains(borderStr, "B") {
		b += "B"
	}
	f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
	f.x = f.lMargin
}

// Output text in flowing mode
func (f *Fpdf) write(h float64, txtStr string, link int, linkStr string) {
	// dbg("Write")
	cw := &f.currentFont.Cw
	w := f.w - f.rMargin - f.x
	wmax := (w - 2*f.cMargin) * 1000 / f.fontSize
	s := strings.Replace(txtStr, "\r", "", -1)
	nb := len(s)
	sep := -1
	i := 0
	j := 0
	l := 0.0
	nl := 1
	for i < nb {
		// 		Get next character
		c := []byte(s)[i]
		if c == '\n' {
			// Explicit line break
			f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			i++
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
			continue
		}
		if c == ' ' {
			sep = i
		}
		l += float64(cw[c])
		if l > wmax {
			// Automatic line break
			if sep == -1 {
				if f.x > f.lMargin {
					// Move to next line
					f.x = f.lMargin
					f.y += h
					w = f.w - f.rMargin - f.x
					wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
					i++
					nl++
					continue
				}
				if i == j {
					i++
				}
				f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			} else {
				f.CellFormat(w, h, s[j:sep], "", 2, "", false, link, linkStr)
				i = sep + 1
			}
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
		} else {
			i++
		}
	}
	// Last chunk
	if i != j {
		f.CellFormat(l/1000*f.fontSize, h, s[j:], "", 0, "", false, link, linkStr)
	}
}

// This method prints text from the current position. When the right margin is
// reached (or the \n character is met) a line break occurs and text continues
// from the left margin. Upon method exit, the current position is left just at
// the end of the text.
//
// It is possible to put a link on the text.
//
// h indicates the line height in the unit of measure specified in New().
func (f *Fpdf) Write(h float64, txtStr string) {
	f.write(h, txtStr, 0, "")
}

// Like Write but uses printf-style formatting. See the documentation for
// package fmt for more details on fmtStr and args.
func (f *Fpdf) Writef(h float64, fmtStr string, args ...interface{}) {
	f.write(h, sprintf(fmtStr, args...), 0, "")
}

// Write text that when clicked launches an external URL. See Write() for
// argument details.
func (f *Fpdf) WriteLinkString(h float64, displayStr, targetStr string) {
	f.write(h, displayStr, 0, targetStr)
}

// Write text that when clicked jumps to another location in the PDF. linkId is
// an identifier returned by AddLink(). See Write() for argument details.
func (f *Fpdf) WriteLinkId(h float64, displayStr string, linkId int) {
	f.write(h, displayStr, linkId, "")
}

// Performs a line break. The current abscissa goes back to the left margin and
// the ordinate increases by the amount passed in parameter. A negative value
// of h indicates the height of the last printed cell.
func (f *Fpdf) Ln(h float64) {
	f.x = f.lMargin
	if h < 0 {
		f.y += f.lasth
	} else {
		f.y += h
	}
}

// Puts a JPEG, PNG or GIF image. The size it will take on the page can be
// specified in different ways. If both w and h are 0, the image is rendered at
// 96 dpi. If either w or h is zero, it will be calculated from the other
// dimension so that the aspect ratio is maintained. If w and h are negative,
// their absolute values indicate their dpi extents.
//
// Supported JPEG formats are 24 bit, 32 bit and gray scale. Supported PNG
// formats are 24 bit, indexed color, and 8 bit indexed gray scale. If a GIF
// image is animated, only the first frame is rendered. Transparency is
// supported. It is possible to put a link on the image. Remark: if an image is
// used several times, only one copy is embedded in the file.
//
// If x is negative, the current abscissa is used.
//
// tp specifies the image format. Possible values are (case insensitive):
// "JPG", "JPEG", "PNG" and "GIF". If not specified, the type is inferred from
// the file extension.
func (f *Fpdf) Image(fileStr string, x, y, w, h float64, flow bool, tp string, link int, linkStr string) {
	if f.err != nil {
		return
	}
	var info imageInfoType
	var ok bool
	info, ok = f.images[fileStr]
	if !ok {
		// First use of this image, get info
		if tp == "" {
			pos := strings.LastIndex(fileStr, ".")
			if pos < 0 {
				f.err = fmt.Errorf("Image file has no extension and no type was specified: %s", fileStr)
				return
			}
			tp = fileStr[pos+1:]
		}
		tp = strings.ToLower(tp)
		if tp == "jpeg" {
			tp = "jpg"
		}
		switch tp {
		case "jpg":
			info = f.parsejpg(fileStr)
		case "png":
			info = f.parsepng(fileStr)
		case "gif":
			info = f.parsegif(fileStr)
		default:
			f.err = fmt.Errorf("Unsupported image type: %s", tp)
		}
		if f.err != nil {
			return
		}
		info.i = len(f.images) + 1
		f.images[fileStr] = info
	}
	// Automatic width and height calculation if needed
	if w == 0 && h == 0 {
		// Put image at 96 dpi
		w = -96
		h = -96
	}
	if w < 0 {
		w = -info.w * 72.0 / w / f.k
	}
	if h < 0 {
		h = -info.h * 72.0 / h / f.k
	}
	if w == 0 {
		w = h * info.w / info.h
	}
	if h == 0 {
		h = w * info.h / info.w
	}
	// Flowing mode
	if flow {
		if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
			// Automatic page break
			x2 := f.x
			f.AddPageFormat(f.curOrientation, f.curPageSize)
			if f.err != nil {
				return
			}
			f.x = x2
		}
		y = f.y
		f.y += h
	}
	if x < 0 {
		x = f.x
	}
	// dbg("h %.2f", h)
	// q 85.04 0 0 NaN 28.35 NaN cm /I2 Do Q
	f.outf("q %.2f 0 0 %.2f %.2f %.2f cm /I%d Do Q", w*f.k, h*f.k, x*f.k, (f.h-(y+h))*f.k, info.i)
	if link > 0 || len(linkStr) > 0 {
		f.newLink(x, y, w, h, link, linkStr)
	}
	return
}

// Returns the abscissa of the current position.
func (f *Fpdf) GetX() float64 {
	return f.x
}

// Defines the abscissa of the current position. If the passed value is
// negative, it is relative to the right of the page.
func (f *Fpdf) SetX(x float64) {
	if x >= 0 {
		f.x = x
	} else {
		f.x = f.w + x
	}
}

// Returns the ordinate of the current position.
func (f *Fpdf) GetY() float64 {
	return f.y
}

// Moves the current abscissa back to the left margin and sets the ordinate. If
// the passed value is negative, it is relative to the bottom of the page.
func (f *Fpdf) SetY(y float64) {
	// dbg("SetY x %.2f, lMargin %.2f", f.x, f.lMargin)
	f.x = f.lMargin
	if y >= 0 {
		f.y = y
	} else {
		f.y = f.h + y
	}
}

// Defines the abscissa and ordinate of the current position. If the passed
// values are negative, they are relative respectively to the right and bottom
// of the page.
func (f *Fpdf) SetXY(x, y float64) {
	f.SetY(y)
	f.SetX(x)
}

// Send the PDF document to the writer specified by w. This method will close
// both f and w, even if an error is detected and no document is produced.
func (f *Fpdf) OutputAndClose(w io.WriteCloser) error {
	f.Output(w)
	w.Close()
	return f.err
}

// Send the PDF document to the writer specified by w. No output will take
// place if an error has occured in the document generation process. w remains
// open after this function returns. After returning, f is in a closed state
// and its methods should not be called.
func (f *Fpdf) Output(w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	// dbg("Output")
	if f.state < 3 {
		f.Close()
	}
	_, err := f.buffer.WriteTo(w)
	if err != nil {
		f.err = err
	}
	return f.err
}

func (f *Fpdf) getpagesizestr(sizeStr string) (size sizeType) {
	if f.err != nil {
		return
	}
	sizeStr = strings.ToLower(sizeStr)
	// dbg("Size [%s]", sizeStr)
	if sizeStr == "" {
		// dbg("not found %s", sizeStr)
		size = f.defPageSize
	} else {
		var ok bool
		size, ok = f.stdpageSizes[sizeStr]
		if ok {
			// dbg("found %s", sizeStr)
			size.wd /= f.k
			size.ht /= f.k

		} else {
			f.err = fmt.Errorf("Unknown page size %s", sizeStr)
		}
	}
	return
}

func (f *Fpdf) _getpagesize(size sizeType) sizeType {
	if size.wd > size.ht {
		size.wd, size.ht = size.ht, size.wd
	}
	return size
}

func (f *Fpdf) beginpage(orientationStr string, size sizeType) {
	if f.err != nil {
		return
	}
	f.page++
	f.pages = append(f.pages, bytes.NewBufferString(""))
	f.pageLinks = append(f.pageLinks, make([]linkType, 0, 0))
	f.state = 2
	f.x = f.lMargin
	f.y = f.tMargin
	f.fontFamily = ""
	// Check page size and orientation
	if orientationStr == "" {
		orientationStr = f.defOrientation
	} else {
		orientationStr = strings.ToUpper(orientationStr[0:1])
	}
	if orientationStr != f.curOrientation || size.wd != f.curPageSize.wd || size.ht != f.curPageSize.ht {
		// New size or orientation
		if orientationStr == "P" {
			f.w = size.wd
			f.h = size.ht
		} else {
			f.w = size.ht
			f.h = size.wd
		}
		f.wPt = f.w * f.k
		f.hPt = f.h * f.k
		f.pageBreakTrigger = f.h - f.bMargin
		f.curOrientation = orientationStr
		f.curPageSize = size
	}
	if orientationStr != f.defOrientation || size.wd != f.defPageSize.wd || size.ht != f.defPageSize.ht {
		f.pageSizes[f.page] = sizeType{f.wPt, f.hPt}
	}
	return
}

func (f *Fpdf) endpage() {
	f.state = 1
}

// Load a font definition file from the font directory
func (f *Fpdf) loadfont(fontStr string) (def fontDefType) {
	if f.err != nil {
		return
	}
	// dbg("Loading font [%s]", fontStr)
	buf, err := ioutil.ReadFile(fontStr)
	if err != nil {
		f.err = err
		return
	}
	err = json.Unmarshal(buf, &def)
	if err != nil {
		f.err = err
	}
	// dump(def)
	return
}

// Escape special characters in strings
func (f *Fpdf) escape(s string) string {
	s = strings.Replace(s, "\\", "\\\\", -1)
	s = strings.Replace(s, "(", "\\(", -1)
	s = strings.Replace(s, ")", "\\)", -1)
	s = strings.Replace(s, "\r", "\\r", -1)
	return s
}

// Format a text string
func (f *Fpdf) textstring(s string) string {
	return "(" + f.escape(s) + ")"
}

func blankCount(str string) (count int) {
	l := len(str)
	for j := 0; j < l; j++ {
		if byte(' ') == str[j] {
			count++
		}
	}
	return
}

// Underline text
func (f *Fpdf) dounderline(x, y float64, txt string) string {
	up := float64(f.currentFont.Up)
	ut := float64(f.currentFont.Ut)
	w := f.GetStringWidth(txt) + f.ws*float64(blankCount(txt))
	return sprintf("%.2f %.2f %.2f %.2f re f", x*f.k,
		(f.h-(y-up/1000*f.fontSize))*f.k, w*f.k, -ut/1000*f.fontSizePt)
}

func bufEqual(buf []byte, str string) bool {
	return string(buf[0:len(str)]) == str
}

func be16(buf []byte) int {
	return 256*int(buf[0]) + int(buf[1])
}

// Extract info from a JPEG file
// Thank you, Michael Petrov: http://www.64lines.com/jpeg-width-height
func (f *Fpdf) parsejpg(fileStr string) (info imageInfoType) {
	var err error
	info.data, err = ioutil.ReadFile(fileStr)
	if err != nil {
		f.err = err
		return
	}
	if bufEqual(info.data[0:], "\xff\xd8\xff\xe0") && bufEqual(info.data[6:], "JFIF\x00") {
		dataLen := len(info.data)
		pos := 4
		blockLen := be16(info.data[4:])
		loop := true
		for pos+blockLen < dataLen && loop {
			pos += blockLen
			if info.data[pos] != 0xff {
				f.err = fmt.Errorf("Unexpected JPEG segment header: %s\n", fileStr)
				return
			}
			if info.data[pos+1] == 0xc0 {
				// Start-of-frame segment
				info.h = float64(be16(info.data[pos+5:]))
				info.w = float64(be16(info.data[pos+7:]))
				info.bpc = int(info.data[pos+4])
				compNum := info.data[pos+9]
				switch compNum {
				case 3:
					info.cs = "DeviceRGB"
				case 4:
					info.cs = "DeviceCMYK"
				case 1:
					info.cs = "DeviceGray"
				default:
					f.err = fmt.Errorf("JPEG buffer has unsupported color space (%d)", compNum)
					return
				}
				loop = false
			} else {
				pos += 2
				blockLen = be16(info.data[pos:])
			}
		}
	} else {
		f.err = fmt.Errorf("Improper JPEG header: %s\n", fileStr)
	}
	info.f = "DCTDecode"
	return
}

// Extract info from a PNG file
func (f *Fpdf) parsepng(fileStr string) (info imageInfoType) {
	buf, err := bufferFromFile(fileStr)
	if err != nil {
		f.err = err
		return
	}
	return f.parsepngstream(buf)
}

func (f *Fpdf) readBeInt32(buf *bytes.Buffer) (val int32) {
	err := binary.Read(buf, binary.BigEndian, &val)
	if err != nil {
		f.err = err
	}
	return
}

func (f *Fpdf) readByte(buf *bytes.Buffer) (val byte) {
	err := binary.Read(buf, binary.BigEndian, &val)
	if err != nil {
		f.err = err
	}
	return
}

func (f *Fpdf) parsepngstream(buf *bytes.Buffer) (info imageInfoType) {
	// 	Check signature
	if string(buf.Next(8)) != "\x89PNG\x0d\x0a\x1a\x0a" {
		f.err = fmt.Errorf("Not a PNG buffer")
		return
	}
	// Read header chunk
	_ = buf.Next(4)
	if string(buf.Next(4)) != "IHDR" {
		f.err = fmt.Errorf("Incorrect PNG buffer")
		return
	}
	w := f.readBeInt32(buf)
	h := f.readBeInt32(buf)
	bpc := f.readByte(buf)
	if bpc > 8 {
		f.err = fmt.Errorf("16-bit depth not supported in PNG file")
	}
	ct := f.readByte(buf)
	var colspace string
	colorVal := 1
	switch ct {
	case 0, 4:
		colspace = "DeviceGray"
	case 2, 6:
		colspace = "DeviceRGB"
		colorVal = 3
	case 3:
		colspace = "Indexed"
	default:
		f.err = fmt.Errorf("Unknown color type in PNG buffer: %d", ct)
	}
	if f.err != nil {
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("'Unknown compression method in PNG buffer")
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("'Unknown filter method in PNG buffer")
		return
	}
	if f.readByte(buf) != 0 {
		f.err = fmt.Errorf("Interlacing not supported in PNG buffer")
		return
	}
	_ = buf.Next(4)
	dp := sprintf("/Predictor 15 /Colors %d /BitsPerComponent %d /Columns %d", colorVal, bpc, w)
	// Scan chunks looking for palette, transparency and image data
	pal := make([]byte, 0, 32)
	var trns []int
	data := make([]byte, 0, 32)
	loop := true
	for loop {
		n := int(f.readBeInt32(buf))
		// dbg("Loop [%d]", n)
		switch string(buf.Next(4)) {
		case "PLTE":
			// dbg("PLTE")
			// Read palette
			pal = buf.Next(n)
			_ = buf.Next(4)
		case "tRNS":
			// dbg("tRNS")
			// Read transparency info
			t := buf.Next(n)
			if ct == 0 {
				trns = []int{int(t[1])} // ord(substr($t,1,1)));
			} else if ct == 2 {
				trns = []int{int(t[1]), int(t[3]), int(t[5])} // array(ord(substr($t,1,1)), ord(substr($t,3,1)), ord(substr($t,5,1)));
			} else {
				pos := strings.Index(string(t), "\x00")
				if pos >= 0 {
					trns = []int{pos} // array($pos);
				}
			}
			_ = buf.Next(4)
		case "IDAT":
			// dbg("IDAT")
			// Read image data block
			data = append(data, buf.Next(n)...)
			_ = buf.Next(4)
		case "IEND":
			// dbg("IEND")
			loop = false
		default:
			// dbg("default")
			_ = buf.Next(n + 4)
		}
		if loop {
			loop = n > 0
		}
	}
	if colspace == "Indexed" && len(pal) == 0 {
		f.err = fmt.Errorf("Missing palette in PNG buffer")
	}
	info.w = float64(w)
	info.h = float64(h)
	info.cs = colspace
	info.bpc = int(bpc)
	info.f = "FlateDecode"
	info.dp = dp
	info.pal = pal
	info.trns = trns
	// dbg("ct [%d]", ct)
	if ct >= 4 {
		// Separate alpha and color channels
		var err error
		data, err = sliceUncompress(data)
		if err != nil {
			f.err = err
			return
		}
		var color, alpha bytes.Buffer
		if ct == 4 {
			// Gray image
			width := int(w)
			height := int(h)
			length := 2 * width
			var pos, elPos int
			for i := 0; i < height; i++ {
				pos = (1 + length) * i
				color.WriteByte(data[pos])
				alpha.WriteByte(data[pos])
				elPos = pos + 1
				for k := 0; k < width; k++ {
					color.WriteByte(data[elPos])
					alpha.WriteByte(data[elPos+1])
					elPos += 2
				}
			}
		} else {
			// RGB image
			width := int(w)
			height := int(h)
			length := 4 * width
			var pos, elPos int
			for i := 0; i < height; i++ {
				pos = (1 + length) * i
				color.WriteByte(data[pos])
				alpha.WriteByte(data[pos])
				elPos = pos + 1
				for k := 0; k < width; k++ {
					color.Write(data[elPos : elPos+3])
					alpha.WriteByte(data[elPos+3])
					elPos += 4
				}
			}
		}
		data = sliceCompress(color.Bytes())
		info.smask = sliceCompress(alpha.Bytes())
		if f.pdfVersion < "1.4" {
			f.pdfVersion = "1.4"
		}
	}
	info.data = data
	return
}

// Extract info from a GIF file (via PNG conversion)
func (f *Fpdf) parsegif(fileStr string) (info imageInfoType) {
	data, err := ioutil.ReadFile(fileStr)
	if err != nil {
		f.err = err
		return
	}
	gifBuf := bytes.NewBuffer(data)
	var img image.Image
	img, err = gif.Decode(gifBuf)
	if err != nil {
		f.err = err
		return
	}
	pngBuf := new(bytes.Buffer)
	err = png.Encode(pngBuf, img)
	if err != nil {
		f.err = err
		return
	}
	return f.parsepngstream(pngBuf)
}

// Begin a new object
func (f *Fpdf) newobj() {
	// dbg("newobj")
	f.n++
	for j := len(f.offsets); j <= f.n; j++ {
		f.offsets = append(f.offsets, 0)
	}
	f.offsets[f.n] = f.buffer.Len()
	f.outf("%d 0 obj", f.n)
}

func (f *Fpdf) putstream(b []byte) {
	// dbg("putstream")
	f.out("stream")
	f.out(string(b))
	f.out("endstream")
}

// Add a line to the document
func (f *Fpdf) out(s string) {
	if f.state == 2 {
		f.pages[f.page].WriteString(s)
		f.pages[f.page].WriteString("\n")
	} else {
		f.buffer.WriteString(s)
		f.buffer.WriteString("\n")
	}
}

// Add a buffered line to the document
func (f *Fpdf) outbuf(b *bytes.Buffer) {
	if f.state == 2 {
		f.pages[f.page].ReadFrom(b)
		f.pages[f.page].WriteString("\n")
	} else {
		f.buffer.ReadFrom(b)
		f.buffer.WriteString("\n")
	}
}

// Add a formatted line to the document
func (f *Fpdf) outf(fmtStr string, args ...interface{}) {
	f.out(sprintf(fmtStr, args...))
}

func (f *Fpdf) putpages() {
	var wPt, hPt float64
	var pageSize sizeType
	// var linkList []linkType
	var ok bool
	nb := f.page
	if len(f.aliasNbPagesStr) > 0 {
		// Replace number of pages
		nbStr := sprintf("%d", nb)
		for n := 1; n <= nb; n++ {
			s := f.pages[n].String()
			if strings.Contains(s, f.aliasNbPagesStr) {
				s = strings.Replace(s, f.aliasNbPagesStr, nbStr, -1)
				f.pages[n].Truncate(0)
				f.pages[n].WriteString(s)
			}
		}
	}
	if f.defOrientation == "P" {
		wPt = f.defPageSize.wd * f.k
		hPt = f.defPageSize.ht * f.k
	} else {
		wPt = f.defPageSize.ht * f.k
		hPt = f.defPageSize.wd * f.k
	}
	for n := 1; n <= nb; n++ {
		// Page
		f.newobj()
		f.out("<</Type /Page")
		f.out("/Parent 1 0 R")
		pageSize, ok = f.pageSizes[n]
		if ok {
			f.outf("/MediaBox [0 0 %.2f %.2f]", pageSize.wd, pageSize.ht)
		}
		f.out("/Resources 2 0 R")
		// Links
		if len(f.pageLinks[n]) > 0 {
			var annots fmtBuffer
			annots.printf("/Annots [")
			for _, pl := range f.pageLinks[n] {
				annots.printf("<</Type /Annot /Subtype /Link /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0] ",
					pl.x, pl.y, pl.x+pl.wd, pl.y-pl.ht)
				if pl.link == 0 {
					annots.printf("/A <</S /URI /URI %s>>>>", f.textstring(pl.linkStr))
				} else {
					l := f.links[pl.link]
					var sz sizeType
					var h float64
					sz, ok = f.pageSizes[l.page]
					if ok {
						h = sz.ht
					} else {
						h = hPt
					}
					// dbg("h [%.2f], l.y [%.2f] f.k [%.2f]\n", h, l.y, f.k)
					annots.printf("/Dest [%d 0 R /XYZ 0 %.2f null]>>", 1+2*l.page, h-l.y*f.k)
				}
			}
			annots.printf("]")
			f.out(annots.String())
		}
		if f.pdfVersion > "1.3" {
			f.out("/Group <</Type /Group /S /Transparency /CS /DeviceRGB>>")
		}
		f.outf("/Contents %d 0 R>>", f.n+1)
		f.out("endobj")
		// Page content
		f.newobj()
		if f.compress {
			data := sliceCompress(f.pages[n].Bytes())
			f.outf("<</Filter /FlateDecode /Length %d>>", len(data))
			f.putstream(data)
		} else {
			f.outf("<</Length %d>>", f.pages[n].Len())
			f.putstream(f.pages[n].Bytes())
		}
		f.out("endobj")
	}
	// Pages root
	f.offsets[1] = f.buffer.Len()
	f.out("1 0 obj")
	f.out("<</Type /Pages")
	var kids fmtBuffer
	kids.printf("/Kids [")
	for i := 0; i < nb; i++ {
		kids.printf("%d 0 R ", 3+2*i)
	}
	kids.printf("]")
	f.out(kids.String())
	f.outf("/Count %d", nb)
	f.outf("/MediaBox [0 0 %.2f %.2f]", wPt, hPt)
	f.out(">>")
	f.out("endobj")
}

func (f *Fpdf) putfonts() {
	if f.err != nil {
		return
	}
	nf := f.n
	for _, diff := range f.diffs {
		// Encodings
		f.newobj()
		f.outf("<</Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [%s]>>", diff)
		f.out("endobj")
	}
	for file, info := range f.fontFiles {
		// 	foreach($this->fontFiles as $file=>$info)
		// Font file embedding
		f.newobj()
		info.n = f.n
		f.fontFiles[file] = info
		font, err := ioutil.ReadFile(path.Join(f.fontpath, file))
		if err != nil {
			f.err = err
			return
		}
		// dbg("font file [%s], ext [%s]", file, file[len(file)-2:])
		compressed := file[len(file)-2:] == ".z"
		if !compressed && info.length2 > 0 {
			buf := font[6:info.length1]
			buf = append(buf, font[6+info.length1+6:info.length2]...)
			font = buf
		}
		f.outf("<</Length %d", len(font))
		if compressed {
			f.out("/Filter /FlateDecode")
		}
		f.outf("/Length1 %d", info.length1)
		if info.length2 > 0 {
			f.outf("/Length2 %d /Length3 0", info.length2)
		}
		f.out(">>")
		f.putstream(font)
		f.out("endobj")
	}
	for k, font := range f.fonts {
		// Font objects
		font.N = f.n + 1
		f.fonts[k] = font
		tp := font.Tp
		name := font.Name
		if tp == "Core" {
			// Core font
			f.newobj()
			f.out("<</Type /Font")
			f.outf("/BaseFont /%s", name)
			f.out("/Subtype /Type1")
			if name != "Symbol" && name != "ZapfDingbats" {
				f.out("/Encoding /WinAnsiEncoding")
			}
			f.out(">>")
			f.out("endobj")
		} else if tp == "Type1" || tp == "TrueType" {
			// Additional Type1 or TrueType/OpenType font
			f.newobj()
			f.out("<</Type /Font")
			f.outf("/BaseFont /%s", name)
			f.outf("/Subtype /%s", tp)
			f.out("/FirstChar 32 /LastChar 255")
			f.outf("/Widths %d 0 R", f.n+1)
			f.outf("/FontDescriptor %d 0 R", f.n+2)
			if font.DiffN > 0 {
				f.outf("/Encoding %d 0 R", nf+font.DiffN)
			} else {
				f.out("/Encoding /WinAnsiEncoding")
			}
			f.out(">>")
			f.out("endobj")
			// Widths
			f.newobj()
			var s fmtBuffer
			s.WriteString("[")
			for j := 32; j < 256; j++ {
				s.printf("%d ", font.Cw[j])
			}
			s.WriteString("]")
			f.out(s.String())
			f.out("endobj")
			// Descriptor
			f.newobj()
			s.Truncate(0)
			s.printf("<</Type /FontDescriptor /FontName /%s ", name)
			s.printf("/Ascent %d ", font.Desc.Ascent)
			s.printf("/Descent %d ", font.Desc.Descent)
			s.printf("/CapHeight %d ", font.Desc.CapHeight)
			s.printf("/Flags %d ", font.Desc.Flags)
			s.printf("/FontBBox [%d %d %d %d] ", font.Desc.FontBBox.Xmin, font.Desc.FontBBox.Ymin,
				font.Desc.FontBBox.Xmax, font.Desc.FontBBox.Ymax)
			s.printf("/ItalicAngle %d ", font.Desc.ItalicAngle)
			s.printf("/StemV %d ", font.Desc.StemV)
			s.printf("/MissingWidth %d ", font.Desc.MissingWidth)
			var suffix string
			if tp != "Type1" {
				suffix = "2"
			}
			s.printf("/FontFile%s %d 0 R>>", suffix, f.fontFiles[font.File].n)
			f.out(s.String())
			f.out("endobj")
		} else {
			f.err = fmt.Errorf("Unsupported font type: %s", tp)
			return
			// Allow for additional types
			// 			$mtd = 'put'.strtolower($type);
			// 			if(!method_exists($this,$mtd))
			// 				$this->Error('Unsupported font type: '.$type);
			// 			$this->$mtd($font);
		}
	}
	return
}

func (f *Fpdf) putimages() {
	for fileStr, img := range f.images {
		f.putimage(&img)
		img.data = img.data[0:0]
		img.smask = img.smask[0:0]
		f.images[fileStr] = img
	}
}

func (f *Fpdf) putimage(info *imageInfoType) {
	f.newobj()
	info.n = f.n
	f.out("<</Type /XObject")
	f.out("/Subtype /Image")
	f.outf("/Width %d", int(info.w))
	f.outf("/Height %d", int(info.h))
	if info.cs == "Indexed" {
		f.outf("/ColorSpace [/Indexed /DeviceRGB %d %d 0 R]", len(info.pal)/3-1, f.n+1)
	} else {
		f.outf("/ColorSpace /%s", info.cs)
		if info.cs == "DeviceCMYK" {
			f.out("/Decode [1 0 1 0 1 0 1 0]")
		}
	}
	f.outf("/BitsPerComponent %d", info.bpc)
	if len(info.f) > 0 {
		f.outf("/Filter /%s", info.f)
	}
	if len(info.dp) > 0 {
		f.outf("/DecodeParms <<%s>>", info.dp)
	}
	if len(info.trns) > 0 {
		var trns fmtBuffer
		for _, v := range info.trns {
			trns.printf("%d %d ", v, v)
		}
		f.outf("/Mask [%s]", trns.String())
	}
	if info.smask != nil {
		f.outf("/SMask %d 0 R", f.n+1)
	}
	f.outf("/Length %d>>", len(info.data))
	f.putstream(info.data)
	f.out("endobj")
	// 	Soft mask
	if len(info.smask) > 0 {
		smask := imageInfoType{
			w:    info.w,
			h:    info.h,
			cs:   "DeviceGray",
			bpc:  8,
			f:    info.f,
			dp:   sprintf("/Predictor 15 /Colors 1 /BitsPerComponent 8 /Columns %d", int(info.w)),
			data: info.smask,
		}
		f.putimage(&smask)
	}
	// 	Palette
	if info.cs == "Indexed" {
		f.newobj()
		if f.compress {
			pal := sliceCompress(info.pal)
			f.outf("<</Filter /FlateDecode /Length %d>>", len(pal))
			f.putstream(pal)
		} else {
			f.outf("<</Length %d>>", len(info.pal))
			f.putstream(info.pal)
		}
		f.out("endobj")
	}
}

func (f *Fpdf) putxobjectdict() {
	for _, image := range f.images {
		// 	foreach($this->images as $image)
		f.outf("/I%d %d 0 R", image.i, image.n)
	}
}

func (f *Fpdf) putresourcedict() {
	f.out("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")
	f.out("/Font <<")
	for _, font := range f.fonts {
		// 	foreach($this->fonts as $font)
		f.outf("/F%d %d 0 R", font.I, font.N)
	}
	f.out(">>")
	f.out("/XObject <<")
	f.putxobjectdict()
	f.out(">>")
}

func (f *Fpdf) putresources() {
	if f.err != nil {
		return
	}
	f.putfonts()
	if f.err != nil {
		return
	}
	f.putimages()
	// 	Resource dictionary
	f.offsets[2] = f.buffer.Len()
	f.out("2 0 obj")
	f.out("<<")
	f.putresourcedict()
	f.out(">>")
	f.out("endobj")
	return
}

func (f *Fpdf) putinfo() {
	f.outf("/Producer %s", f.textstring("FPDF "+FPDF_VERSION))
	if len(f.title) > 0 {
		f.outf("/Title %s", f.textstring(f.title))
	}
	if len(f.subject) > 0 {
		f.outf("/Subject %s", f.textstring(f.subject))
	}
	if len(f.author) > 0 {
		f.outf("/Author %s", f.textstring(f.author))
	}
	if len(f.keywords) > 0 {
		f.outf("/Keywords %s", f.textstring(f.keywords))
	}
	if len(f.creator) > 0 {
		f.outf("/Creator %s", f.textstring(f.creator))
	}
	f.outf("/CreationDate %s", f.textstring("D:"+time.Now().Format("20060102150405")))
}

func (f *Fpdf) putcatalog() {
	f.out("/Type /Catalog")
	f.out("/Pages 1 0 R")
	switch f.zoomMode {
	case "fullpage":
		f.out("/OpenAction [3 0 R /Fit]")
	case "fullwidth":
		f.out("/OpenAction [3 0 R /FitH null]")
	case "real":
		f.out("/OpenAction [3 0 R /XYZ null null 1]")
	}
	// } 	else if !is_string($this->zoomMode))
	// 		$this->out('/OpenAction [3 0 R /XYZ null null '.sprintf('%.2f',$this->zoomMode/100).']');
	switch f.layoutMode {
	case "single":
		f.out("/PageLayout /SinglePage")
	case "continuous":
		f.out("/PageLayout /OneColumn")
	case "two":
		f.out("/PageLayout /TwoColumnLeft")
	}
}

func (f *Fpdf) putheader() {
	f.outf("%%PDF-%s", f.pdfVersion)
}

func (f *Fpdf) puttrailer() {
	f.outf("/Size %d", f.n+1)
	f.outf("/Root %d 0 R", f.n)
	f.outf("/Info %d 0 R", f.n-1)
}

func (f *Fpdf) enddoc() {
	if f.err != nil {
		return
	}
	f.putheader()
	f.putpages()
	f.putresources()
	if f.err != nil {
		return
	}
	// 	Info
	f.newobj()
	f.out("<<")
	f.putinfo()
	f.out(">>")
	f.out("endobj")
	// 	Catalog
	f.newobj()
	f.out("<<")
	f.putcatalog()
	f.out(">>")
	f.out("endobj")
	// Cross-ref
	o := f.buffer.Len()
	f.out("xref")
	f.outf("0 %d", f.n+1)
	f.out("0000000000 65535 f ")
	for j := 1; j <= f.n; j++ {
		f.outf("%010d 00000 n ", f.offsets[j])
	}
	// Trailer
	f.out("trailer")
	f.out("<<")
	f.puttrailer()
	f.out(">>")
	f.out("startxref")
	f.outf("%d", o)
	f.out("%%EOF")
	f.state = 3
	return
}