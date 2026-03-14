package report

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type cellKind string

const (
	cellString cellKind = "string"
	cellNumber cellKind = "number"
	cellDate   cellKind = "date"
)

type cellValue struct {
	kind  cellKind
	value string
}

func WriteWorkbook(path string, sheetName string, rows [][]string) error {
	if len(rows) == 0 {
		return fmt.Errorf("cannot write workbook without rows")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create workbook: %w", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)

	colWidths, parsedRows := analyzeRows(rows)

	if err := writeZipFile(zipWriter, "[Content_Types].xml", contentTypesXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "_rels/.rels", rootRelationshipsXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "docProps/app.xml", appPropsXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "docProps/core.xml", corePropsXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "xl/workbook.xml", workbookXML(sheetName)); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "xl/_rels/workbook.xml.rels", workbookRelationshipsXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "xl/styles.xml", stylesXML); err != nil {
		return err
	}

	if err := writeZipFile(zipWriter, "xl/worksheets/sheet1.xml", worksheetXML(parsedRows, colWidths)); err != nil {
		return err
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close workbook archive: %w", err)
	}

	return nil
}

func analyzeRows(rows [][]string) ([]float64, [][]cellValue) {
	widths := make([]float64, len(rows[0]))
	parsed := make([][]cellValue, 0, len(rows))
	headers := rows[0]

	for rowIndex, row := range rows {
		parsedRow := make([]cellValue, len(row))

		for colIndex, value := range row {
			widths[colIndex] = math.Max(widths[colIndex], columnWidth(value))

			if rowIndex == 0 {
				parsedRow[colIndex] = cellValue{kind: cellString, value: value}
				continue
			}

			header := headers[colIndex]
			switch {
			case isDateHeader(header):
				if parsedDate, ok := parseDate(value); ok {
					parsedRow[colIndex] = cellValue{kind: cellDate, value: excelDate(parsedDate)}
					continue
				}
			case isNumberHeader(header):
				if parsedNumber, ok := parseNumber(value); ok {
					parsedRow[colIndex] = cellValue{kind: cellNumber, value: strconv.FormatFloat(parsedNumber, 'f', 2, 64)}
					continue
				}
			}

			parsedRow[colIndex] = cellValue{kind: cellString, value: value}
		}

		parsed = append(parsed, parsedRow)
	}

	for index := range widths {
		if widths[index] < 12 {
			widths[index] = 12
		}
		if widths[index] > 40 {
			widths[index] = 40
		}
	}

	return widths, parsed
}

func worksheetXML(rows [][]cellValue, widths []float64) string {
	var out strings.Builder

	out.WriteString(xml.Header)
	out.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	out.WriteString(fmt.Sprintf(`<dimension ref="A1:%s%d"/>`, columnName(len(widths)), len(rows)))
	out.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	out.WriteString(`<sheetFormatPr defaultRowHeight="15"/>`)

	out.WriteString(`<cols>`)
	for index, width := range widths {
		column := index + 1
		out.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%.2f" customWidth="1"/>`, column, column, width))
	}
	out.WriteString(`</cols>`)

	out.WriteString(`<sheetData>`)
	for rowIndex, row := range rows {
		out.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex+1))
		for colIndex, cell := range row {
			ref := cellRef(colIndex+1, rowIndex+1)
			style := cellStyleIndex(rowIndex, cell.kind)
			switch cell.kind {
			case cellNumber, cellDate:
				out.WriteString(fmt.Sprintf(`<c r="%s" s="%d"><v>%s</v></c>`, ref, style, xmlEscape(cell.value)))
			default:
				out.WriteString(fmt.Sprintf(`<c r="%s" s="%d" t="inlineStr"><is><t>%s</t></is></c>`, ref, style, xmlEscape(cell.value)))
			}
		}
		out.WriteString(`</row>`)
	}
	out.WriteString(`</sheetData>`)

	lastRow := len(rows)
	out.WriteString(fmt.Sprintf(`<autoFilter ref="A1:%s%d"/>`, columnName(len(widths)), lastRow))
	out.WriteString(`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>`)
	out.WriteString(`</worksheet>`)

	return out.String()
}

func workbookXML(sheetName string) string {
	return xml.Header + fmt.Sprintf(
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><bookViews><workbookView xWindow="240" yWindow="15" windowWidth="16095" windowHeight="9660"/></bookViews><sheets><sheet name="%s" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		xmlEscape(sheetName),
	)
}

func writeZipFile(zipWriter *zip.Writer, name string, content string) error {
	file, err := zipWriter.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}

	if _, err := file.Write([]byte(content)); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	return nil
}

func xmlEscape(value string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
		return value
	}

	return buffer.String()
}

func cellStyleIndex(rowIndex int, kind cellKind) int {
	if rowIndex == 0 {
		return 1
	}

	switch kind {
	case cellDate:
		return 2
	case cellNumber:
		return 3
	default:
		return 0
	}
}

func cellRef(col, row int) string {
	return fmt.Sprintf("%s%d", columnName(col), row)
}

func columnName(col int) string {
	name := ""
	for col > 0 {
		col--
		name = string(rune('A'+(col%26))) + name
		col /= 26
	}
	return name
}

func columnWidth(value string) float64 {
	return math.Min(40, float64(len(value))+2)
}

func isDateHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "datum", "date":
		return true
	default:
		return false
	}
}

func isNumberHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "betrag", "amount", "sum":
		return true
	default:
		return false
	}
}

func parseDate(value string) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}

func parseNumber(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}

	return parsed, true
}

func excelDate(value time.Time) string {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	serial := value.Sub(base).Hours() / 24
	return strconv.FormatFloat(serial, 'f', 0, 64)
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`

const rootRelationshipsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const workbookRelationshipsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const appPropsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>beantool</Application>
</Properties>`

const corePropsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:creator>beantool</dc:creator>
  <cp:lastModifiedBy>beantool</cp:lastModifiedBy>
</cp:coreProperties>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <numFmts count="1">
    <numFmt numFmtId="164" formatCode="yyyy-mm-dd"/>
  </numFmts>
  <fonts count="2">
    <font>
      <sz val="11"/>
      <name val="Calibri"/>
      <family val="2"/>
    </font>
    <font>
      <b/>
      <sz val="11"/>
      <name val="Calibri"/>
      <family val="2"/>
    </font>
  </fonts>
  <fills count="3">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
    <fill>
      <patternFill patternType="solid">
        <fgColor rgb="FFE7F3FF"/>
        <bgColor indexed="64"/>
      </patternFill>
    </fill>
  </fills>
  <borders count="2">
    <border>
      <left/><right/><top/><bottom/><diagonal/>
    </border>
    <border>
      <left style="thin"/><right style="thin"/><top style="thin"/><bottom style="thin"/><diagonal/>
    </border>
  </borders>
  <cellStyleXfs count="1">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
  </cellStyleXfs>
  <cellXfs count="4">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1"/>
    <xf numFmtId="0" fontId="1" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
    <xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
    <xf numFmtId="4" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
  </cellXfs>
  <cellStyles count="1">
    <cellStyle name="Normal" xfId="0" builtinId="0"/>
  </cellStyles>
</styleSheet>`
