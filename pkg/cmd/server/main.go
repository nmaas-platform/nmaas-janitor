package main

import (
	"fmt"
	"os"

	"gitlab.software.geant.org/nmaas/nmaas-janitor/pkg/api/cmd"
)

func main() {
	if err := cmd.RunServer(); err != nil {
		_, err = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
