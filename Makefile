.PHONY: spike

# T-005 throwaway PDF-engine spike (see internal/pdf/spike/README.md). Writes PDFs to
# internal/pdf/spike/out/ and screenshots to docs/adr/0002-assets/.
SPIKE = go test -tags spike -count=1 -v ./internal/pdf/spike -run
PDFCPU = github.com/pdfcpu/pdfcpu/cmd/pdfcpu@v0.16.1

spike:
	$(SPIKE) 'TestFixtures|TestSplitRuns'
	$(SPIKE) 'TestFpdf_|TestGopdf_'
	@echo "--- C5 benchmark (one process per library, machine should be idle)"
	@$(SPIKE) TestBenchFpdf | grep -E 'BENCH|FAIL'
	@$(SPIKE) TestBenchGopdf | grep -E 'BENCH|FAIL'
	@echo "--- C7 pdfcpu strict validation"
	@go install $(PDFCPU)
	@for f in internal/pdf/spike/out/*.pdf; do "$$(go env GOPATH)/bin/pdfcpu" validate --mode strict "$$f" || exit 1; done
