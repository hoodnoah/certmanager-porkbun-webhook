package main

import (
	// stdlib

	"log"
	"os"

	// internal
	"github.com/hoodnoah/certmanager-porkbun-webhook/internal/solver"

	// external -- kubernetes

	// external -- cert-manager

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// set up a logger
	logger := log.New(os.Stdout, "", log.LstdFlags)

	// create an instance of our solver to be submitted to the webhookserver
	porkbunSolverInstance := solver.NewPorkbunSolver(logger, "porkbun-solver")

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		porkbunSolverInstance,
	)
}
