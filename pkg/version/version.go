package version

import (
	"fmt"
	"io"
	"text/tabwriter"
)

var (
	//Tag is the tagged version e.g. v0.0.1
	Tag = "dev"
	//Time of build
	Time string
	//User that built the package
	User string
	//Commit hash
	Commit string
)

// GetVersion returns version string
func GetVersion() string {
	return Tag + "-" + Time + ":" + User
}

func Print(w io.Writer) error {
	tw := new(tabwriter.Writer)
	tw.Init(w, 0, 0, 0, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "cascade:", "\t", "Release manager")
	fmt.Fprintln(tw, "Version:", "\t", Tag)
	fmt.Fprintln(tw, "Build Commit Hash:", "\t", Commit)
	fmt.Fprintln(tw, "Build Time:", "\t", Time)
	fmt.Fprintln(tw, "Build User:", "\t", User)
	fmt.Fprintln(tw, "Info:", "\t", "https://github.com/goliatone/cascade")
	fmt.Fprintln(tw)
	return tw.Flush()
}
