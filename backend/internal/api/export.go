package api

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"net/http"
	"strconv"

	"backend/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExportMatchedJobsXLSX returns a user-scoped workbook with a ranked job
// tracker and a formula-driven applied/not-applied dashboard pie chart.
func ExportMatchedJobsXLSX(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var matches []repository.JobMatch
	if err := db.Where("user_id = ?", userID).Order("match_score DESC, created_at DESC").Find(&matches).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export matched jobs"})
		return
	}
	var rows [][]string
	rows = append(rows, []string{"Company", "Job title", "Job link", "Apply link", "Location", "Resume fit (%)", "Important skills asked", "Profile skills matched", "Application status", "Last confirmed", "Source"})
	for _, match := range matches {
		var job repository.Job
		if err := db.First(&job, match.JobID).Error; err != nil {
			continue
		}
		var source repository.AllowedSource
		if job.SourceID != 0 {
			_ = db.First(&source, job.SourceID).Error
		}
		status := match.Status
		if status == "" || status == "matched" {
			status = "not_applied"
		}
		rows = append(rows, []string{job.Company, job.Title, job.URL, job.ApplyURL, job.Location,
			strconv.Itoa(match.MatchScore), match.RequiredSkills, match.MatchedSkills, status, job.LastSeenAt.UTC().Format("2006-01-02 15:04 UTC"), source.Name})
	}
	applied, notApplied := 0, 0
	for _, row := range rows[1:] {
		if row[8] == "applied" {
			applied++
		} else {
			notApplied++
		}
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="jobhunter-tracker.xlsx"`)
	if err := writeJobWorkbook(c.Writer, rows, applied, notApplied); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}

func writeJobWorkbook(out http.ResponseWriter, rows [][]string, applied, notApplied int) error {
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	entries := map[string]string{
		"[Content_Types].xml":                 `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/drawings/drawing1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawing+xml"/><Override PartName="/xl/charts/chart1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.chart+xml"/></Types>`,
		"_rels/.rels":                         `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":                     `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Jobs" sheetId="1" r:id="rId1"/><sheet name="Dashboard" sheetId="2" r:id="rId2"/></sheets><calcPr calcMode="auto"/></workbook>`,
		"xl/_rels/workbook.xml.rels":          `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml":                       `<?xml version="1.0" encoding="UTF-8"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF4F46E5"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`,
		"xl/worksheets/sheet1.xml":            worksheetXML(rows),
		"xl/worksheets/sheet2.xml":            dashboardXML(applied, notApplied),
		"xl/worksheets/_rels/sheet2.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/></Relationships>`,
		"xl/drawings/drawing1.xml":            `<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><xdr:twoCellAnchor><xdr:from><xdr:col>3</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>1</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from><xdr:to><xdr:col>10</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>18</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:to><xdr:graphicFrame><xdr:nvGraphicFramePr><xdr:cNvPr id="2" name="Application status pie chart"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr><xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/></xdr:twoCellAnchor></xdr:wsDr>`,
		"xl/drawings/_rels/drawing1.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart1.xml"/></Relationships>`,
		"xl/charts/chart1.xml":                `<?xml version="1.0" encoding="UTF-8"?><c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><c:chart><c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr/></a:pPr><a:r><a:rPr lang="en-US"/><a:t>Application status</a:t></a:r><a:endParaRPr lang="en-US"/></a:p></c:rich></c:tx><c:layout/><c:overlay val="0"/></c:title><c:plotArea><c:layout/><c:pieChart><c:varyColors val="1"/><c:ser><c:idx val="0"/><c:order val="0"/><c:cat><c:strRef><c:f>Dashboard!$A$3:$A$4</c:f></c:strRef></c:cat><c:val><c:numRef><c:f>Dashboard!$B$3:$B$4</c:f></c:numRef></c:val></c:ser><c:firstSliceAng val="0"/></c:pieChart></c:plotArea><c:legend><c:legendPos val="r"/><c:layout/></c:legend><c:plotVisOnly val="1"/><c:dispBlanksAs val="zero"/><c:showDLblsOverMax val="0"/></c:chart></c:chartSpace>`,
	}
	for name, content := range entries {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		if _, err = f.Write([]byte(content)); err != nil {
			return err
		}
	}
	if err := z.Close(); err != nil {
		return err
	}
	_, err := out.Write(buffer.Bytes())
	return err
}

func worksheetXML(rows [][]string) string {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols><col min="1" max="1" width="26" customWidth="1"/><col min="2" max="2" width="32" customWidth="1"/><col min="3" max="4" width="42" customWidth="1"/><col min="5" max="5" width="24" customWidth="1"/><col min="6" max="6" width="14" customWidth="1"/><col min="7" max="8" width="34" customWidth="1"/><col min="9" max="11" width="22" customWidth="1"/></cols><sheetData>`)
	for ri, row := range rows {
		b.WriteString(`<row r="` + strconv.Itoa(ri+1) + `">`)
		for ci, value := range row {
			ref := columnName(ci+1) + strconv.Itoa(ri+1)
			style := ""
			if ri == 0 {
				style = ` s="1"`
			}
			b.WriteString(`<c r="` + ref + `" t="inlineStr"` + style + `><is><t xml:space="preserve">`)
			_ = xml.EscapeText(&b, []byte(value))
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData><autoFilter ref="A1:K` + strconv.Itoa(len(rows)) + `"/></worksheet>`)
	return b.String()
}

func dashboardXML(applied, notApplied int) string {
	return `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Application dashboard</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>Counts update from the Jobs tab statuses when opened in Excel or Google Sheets.</t></is></c></row><row r="3"><c r="A3" t="inlineStr"><is><t>Applied</t></is></c><c r="B3"><f>COUNTIF(Jobs!I2:I` + strconv.Itoa(max(2, applied+notApplied+1)) + `,"applied")</f><v>` + strconv.Itoa(applied) + `</v></c></row><row r="4"><c r="A4" t="inlineStr"><is><t>Not applied</t></is></c><c r="B4"><f>COUNTIF(Jobs!I2:I` + strconv.Itoa(max(2, applied+notApplied+1)) + `,"not_applied")+COUNTIF(Jobs!I2:I` + strconv.Itoa(max(2, applied+notApplied+1)) + `,"matched")</f><v>` + strconv.Itoa(notApplied) + `</v></c></row><row r="5"><c r="A5" t="inlineStr"><is><t>Note: Application status is user-reported and is not verified by employers.</t></is></c></row></sheetData><drawing r:id="rId1"/></worksheet>`
}

func columnName(n int) string {
	name := ""
	for n > 0 {
		n--
		name = string(rune('A'+n%26)) + name
		n /= 26
	}
	return name
}
