package tooling

import "github.com/pterm/pterm"

func NewSpinner(multi *pterm.MultiPrinter, progressText string) *pterm.SpinnerPrinter {
	spinner := &pterm.DefaultSpinner
	if multi != nil {
		spinner = spinner.WithWriter(multi.NewWriter())
	}
	spinner, _ = spinner.WithSequence("▁▁", "▂▂", "▃▃", "▄▄", "▅▅", "▆▆", "▇▇", "██", "▇▇", "▆▆", "▅▅", "▄▄", "▃▃", "▂▂", "▁▁").Start(progressText)
	spinner.SuccessPrinter = pterm.Success.WithPrefix(pterm.Prefix{
		Text:  "DONE",
		Style: &pterm.ThemeDefault.SuccessPrefixStyle,
	})
	return spinner
}
