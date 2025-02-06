package main

import (
	// stdlib
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	// external
	cmacme "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	// internal

	porkbunsolver "github.com/hoodnoah/certmanager-porkbun-webhook/internal/PorkBunSolver"
)

func main() {
	// set up logging
	zapLog, err := zap.NewProduction()
	if err != nil {
		fmt.Println("Failed to initalize logging: %v", err)
		os.Exit(1)
	}
	logger := zapr.NewLogger(zapLog)

	// create solver
	solver := porkbunsolver.NewPorkBunSolver(logger, nil)

	// start webhook server
	if err := startWebhookServer(solver, logger); err != nil {
		logger.Error(err, "Webhook server failed")
		os.Exit(1)
	}
}

func startWebhookServer(solver *porkbunsolver.PorkBunSolver, logger logr.Logger) error {
	// create a new webhook server
	server := &http.Server{
		Addr:           ":8443",
		Handler:        setupRouter(solver),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	logger.Info("Webhook server listening on :8443")

	return server.ListenAndServeTLS("/Users/noahhood/Documents/Repositories/certmanager-porkbun-webook/tls/tls.cert", "/Users/noahhood/Documents/Repositories/certmanager-porkbun-webook/tls/tls.key")
}

func setupRouter(solver *porkbunsolver.PorkBunSolver) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/present", func(w http.ResponseWriter, r *http.Request) {
		handlePresent(w, r, solver)
	})

	mux.HandleFunc("/cleanup", func(w http.ResponseWriter, r *http.Request) {
		handleCleanup(w, r, solver)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	return mux
}

func handlePresent(w http.ResponseWriter, r *http.Request, solver *porkbunsolver.PorkBunSolver) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}

	var req cmacme.ChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Failed to decode request: %v", err), http.StatusBadRequest)
		return
	}

	if err := solver.Present(&req); err != nil {
		http.Error(w, fmt.Sprintf("failed to present challenge: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleCleanup(w http.ResponseWriter, r *http.Request, solver *porkbunsolver.PorkBunSolver) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}

	var req cmacme.ChallengeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Failed to decode request: %v", err), http.StatusBadRequest)
		return
	}

	if err := solver.CleanUp(&req); err != nil {
		http.Error(w, fmt.Sprintf("failed to clean up challenge: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
