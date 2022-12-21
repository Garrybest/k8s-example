package main

import (
	"os"

	apiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	scheduletest "github.com/Garrybest/k8s-example/cmd/schedtest"
)

func main() {
	ctx := apiserver.SetupSignalContext()
	// modify your example app command here
	cmd := scheduletest.NewCommand(ctx)
	code := cli.Run(cmd)
	os.Exit(code)
}
