package report

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/heythisissud/ip-sakti-backend/pkg/domain"
	"github.com/jung-kurt/gofpdf"
)

// GeneratePDF creates a formatted PDF report from an IPRoadmap and returns the bytes.
func GeneratePDF(sessionID string, roadmap *domain.IPRoadmap) ([]byte, error) {
	if roadmap == nil {
		return nil, fmt.Errorf("roadmap is nil")
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AliasNbPages("{nb}")

	tr := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		pdf.CellFormat(0, 10, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "C", false, 0, "")
	})

	pdf.AddPage()

	// ── PAGE 1: HEADER ────────────────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(27, 67, 50) // Dark Green #1B4332
	pdf.CellFormat(0, 10, "IP-SAKTI", "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 12)
	pdf.SetTextColor(102, 102, 102) // Gray
	pdf.CellFormat(0, 6, "Ayurvedic Innovation & IP Navigator", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	pdf.SetDrawColor(200, 200, 200)
	pdf.SetLineWidth(0.5)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetTextColor(211, 47, 47) // Red
	pdf.CellFormat(0, 4, tr("CONFIDENTIAL — FOR INFORMATIONAL PURPOSES ONLY"), "", 1, "L", false, 0, "")

	sessShort := sessionID
	if len(sessShort) > 8 {
		sessShort = sessShort[:8]
	}
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(102, 102, 102)
	headerMeta := fmt.Sprintf("Generated: %s | Session: %s", time.Now().UTC().Format(time.RFC3339), sessShort)
	pdf.CellFormat(0, 4, tr(headerMeta), "", 1, "L", false, 0, "")
	pdf.Ln(6)

	// ── PAGE 1: PRODUCT ASSESSMENT BOX ───────────────────────────────────────
	blocks := int(math.Round(roadmap.OverallConfidence * 10))
	if blocks > 10 {
		blocks = 10
	}
	if blocks < 0 {
		blocks = 0
	}
	confBar := fmt.Sprintf("[%s%s]", strings.Repeat("#", blocks), strings.Repeat("-", 10-blocks))

	pdf.SetFillColor(244, 246, 245)
	pdf.SetDrawColor(200, 215, 205)
	pdf.SetLineWidth(0.4)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(27, 67, 50)
	pdf.MultiCell(0, 6, tr("PRODUCT ASSESSMENT"), "LTR", "L", true)

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(50, 50, 50)
	classStr := fmt.Sprintf("Classification: %s", roadmap.Classification)
	pdf.MultiCell(0, 5, tr(classStr), "LR", "L", true)

	confStr := fmt.Sprintf("Overall Confidence: %.1f%%  %s", roadmap.OverallConfidence*100, confBar)
	pdf.MultiCell(0, 5, tr(confStr), "LR", "L", true)

	if roadmap.ProductSummary != "" {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "I", 9)
		sumStr := fmt.Sprintf("Product Summary: %s", roadmap.ProductSummary)
		pdf.MultiCell(0, 5, tr(sumStr), "LRB", "L", true)
	} else {
		pdf.MultiCell(0, 2, "", "LRB", "L", true)
	}
	pdf.Ln(6)

	// ── PAGE 1+: DOMAIN ANALYSES ──────────────────────────────────────────────
	if len(roadmap.Domains) > 0 {
		pdf.SetFont("Helvetica", "B", 14)
		pdf.SetTextColor(27, 67, 50)
		pdf.CellFormat(0, 8, tr("DOMAIN ANALYSES"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		for _, dom := range roadmap.Domains {
			statusUpper := strings.ToUpper(string(dom.Status))
			headerTitle := fmt.Sprintf("%s — %s", strings.ToUpper(string(dom.Domain)), statusUpper)

			switch dom.Status {
			case domain.StatusRelevant:
				pdf.SetTextColor(46, 125, 50) // Green
			case domain.StatusInsufficientEvidence:
				pdf.SetTextColor(230, 81, 0) // Orange
			default:
				pdf.SetTextColor(117, 117, 117) // Gray
			}
			pdf.SetFont("Helvetica", "B", 12)
			pdf.CellFormat(0, 7, tr(headerTitle), "", 1, "L", false, 0, "")

			pdf.SetFont("Helvetica", "", 9)
			pdf.SetTextColor(100, 100, 100)
			pdf.CellFormat(0, 5, fmt.Sprintf("Confidence: %.1f%%", dom.Confidence*100), "", 1, "L", false, 0, "")

			if dom.NeedsEscalation {
				pdf.SetFont("Helvetica", "B", 9)
				pdf.SetTextColor(211, 47, 47)
				pdf.CellFormat(0, 5, tr("[!] PROFESSIONAL REVIEW REQUIRED"), "", 1, "L", false, 0, "")
			}

			if dom.Finding != "" {
				pdf.SetFont("Helvetica", "", 10)
				pdf.SetTextColor(40, 40, 40)
				pdf.MultiCell(0, 5, tr(dom.Finding), "", "L", false)
			}

			if len(dom.KeyRisks) > 0 {
				pdf.Ln(1)
				pdf.SetFont("Helvetica", "B", 9)
				pdf.SetTextColor(40, 40, 40)
				pdf.CellFormat(0, 5, tr("Key Risks:"), "", 1, "L", false, 0, "")
				pdf.SetFont("Helvetica", "", 9)
				for _, risk := range dom.KeyRisks {
					if strings.TrimSpace(risk) != "" {
						pdf.MultiCell(0, 5, tr(fmt.Sprintf("  - %s", risk)), "", "L", false)
					}
				}
			}

			if len(dom.Citations) > 0 {
				pdf.Ln(1)
				pdf.SetFont("Helvetica", "B", 9)
				pdf.SetTextColor(40, 40, 40)
				pdf.CellFormat(0, 5, tr("Citations:"), "", 1, "L", false, 0, "")
				pdf.SetFont("Helvetica", "", 9)
				pdf.SetTextColor(30, 80, 160)
				for _, cit := range dom.Citations {
					citLine := fmt.Sprintf("  - %s | %s | %s", cit.DocTitle, cit.Section, cit.SourceURL)
					if cit.SourceURL != "" {
						pdf.WriteLinkString(5, tr(citLine)+"\n", cit.SourceURL)
					} else {
						pdf.MultiCell(0, 5, tr(citLine), "", "L", false)
					}
				}
			}

			pdf.Ln(4)
		}
	}

	// ── PAGE N: JURISDICTION NOTES ────────────────────────────────────────────
	if len(roadmap.JurisdictionNotes) > 0 {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "B", 14)
		pdf.SetTextColor(27, 67, 50)
		pdf.CellFormat(0, 8, tr("JURISDICTION NOTES"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		for _, jnote := range roadmap.JurisdictionNotes {
			pdf.SetFont("Helvetica", "B", 10)
			pdf.SetTextColor(40, 40, 40)
			pdf.CellFormat(0, 5, tr(fmt.Sprintf("Market: %s", strings.ToUpper(jnote.Market))), "", 1, "L", false, 0, "")

			if jnote.Note != "" {
				pdf.SetFont("Helvetica", "", 9)
				pdf.MultiCell(0, 5, tr(jnote.Note), "", "L", false)
			}

			reqSep := "NO"
			if jnote.RequiresSeparateAnalysis {
				reqSep = "YES"
			}
			pdf.SetFont("Helvetica", "I", 9)
			pdf.SetTextColor(100, 100, 100)
			pdf.CellFormat(0, 5, tr(fmt.Sprintf("Requires separate analysis: %s", reqSep)), "", 1, "L", false, 0, "")
			pdf.Ln(3)
		}
	}

	// ── PAGE N: ACTION PLAN ───────────────────────────────────────────────────
	if len(roadmap.NextSteps) > 0 {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "B", 14)
		pdf.SetTextColor(27, 67, 50)
		pdf.CellFormat(0, 8, tr("ACTION PLAN"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(40, 40, 40)
		for i, step := range roadmap.NextSteps {
			if strings.TrimSpace(step) != "" {
				stepText := fmt.Sprintf("%d. %s", i+1, step)
				pdf.MultiCell(0, 5, tr(stepText), "", "L", false)
				pdf.Ln(1)
			}
		}
	}

	// ── PAGE N: PROFESSIONAL CONSULTATION REQUIRED ────────────────────────────
	if len(roadmap.HumanEscalation) > 0 {
		pdf.Ln(4)
		pdf.SetFont("Helvetica", "B", 14)
		pdf.SetTextColor(27, 67, 50)
		pdf.CellFormat(0, 8, tr("PROFESSIONAL CONSULTATION REQUIRED"), "", 1, "L", false, 0, "")
		pdf.Ln(2)

		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(230, 235, 230)
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(45, 6, tr("Professional Type"), "1", 0, "L", true, 0, "")
		pdf.CellFormat(95, 6, tr("Reason"), "1", 0, "L", true, 0, "")
		pdf.CellFormat(30, 6, tr("Urgency"), "1", 1, "L", true, 0, "")

		pdf.SetFont("Helvetica", "", 9)
		for _, item := range roadmap.HumanEscalation {
			var rCol, gCol, bCol int
			switch strings.ToLower(item.Urgency) {
			case "before_filing":
				rCol, gCol, bCol = 211, 47, 47 // Red
			case "before_commercialization":
				rCol, gCol, bCol = 230, 81, 0 // Orange
			default:
				rCol, gCol, bCol = 117, 117, 117 // Gray
			}

			lines := pdf.SplitText(tr(item.Reason), 93)
			h := float64(len(lines) * 5)
			if h < 6 {
				h = 6
			}

			yStart := pdf.GetY()
			if yStart+h > 270 {
				pdf.AddPage()
				yStart = pdf.GetY()
			}

			pdf.Rect(20, yStart, 45, h, "D")
			pdf.Rect(65, yStart, 95, h, "D")
			pdf.Rect(160, yStart, 30, h, "D")

			pdf.SetTextColor(40, 40, 40)
			pdf.SetXY(21, yStart+1)
			pdf.MultiCell(43, 4, tr(item.ProfType), "", "L", false)

			pdf.SetXY(66, yStart+1)
			pdf.MultiCell(93, 4, tr(item.Reason), "", "L", false)

			pdf.SetTextColor(rCol, gCol, bCol)
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetXY(161, yStart+1)
			pdf.MultiCell(28, 4, tr(item.Urgency), "", "L", false)

			pdf.SetY(yStart + h)
		}
		pdf.Ln(4)
	}

	// ── LAST PAGE: DISCLAIMER ─────────────────────────────────────────────────
	pdf.Ln(4)
	pdf.SetDrawColor(200, 200, 200)
	pdf.SetLineWidth(0.5)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(0, 5, tr("DISCLAIMER & METADATA"), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)

	totalCitations := 0
	for _, d := range roadmap.Domains {
		totalCitations += len(d.Citations)
	}

	discText := roadmap.Disclaimer
	if discText == "" {
		discText = "This output is for informational purposes only and does not constitute legal advice."
	}
	pdf.MultiCell(0, 4, tr(discText), "", "L", false)
	pdf.Ln(2)

	pdf.MultiCell(0, 4, tr("This report was generated by IP-SAKTI using AI-assisted analysis..."), "", "L", false)
	pdf.MultiCell(0, 4, tr(fmt.Sprintf("Sources retrieved: %d", totalCitations)), "", "L", false)
	pdf.MultiCell(0, 4, tr(fmt.Sprintf("Report generated: %s", time.Now().UTC().Format(time.RFC3339))), "", "L", false)
	pdf.MultiCell(0, 4, tr("IP-SAKTI is not a substitute for qualified legal professionals."), "", "L", false)

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to render PDF: %w", err)
	}

	return buf.Bytes(), nil
}
