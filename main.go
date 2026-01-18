package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

/*
=============================
 Alertmanager Payload Models
=============================
*/

type AlertmanagerPayload struct {
	Alerts []Alert `json:"alerts"`
}

type Alert struct {
	Status      string            `json:"status"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

/*
=============================
 JSON Log Model
=============================
*/

type JSONLog struct {
	Timestamp string `json:"ts"`
	IP        string `json:"ip"`
	Hostname  string `json:"hname"`
	KPI       string `json:"kpi"`
	Value     string `json:"value"`
	Count     string `json:"cnt"`
	Summary   string `json:"app_sub_name"`
}

/*
=============================
 Main
=============================
*/

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/alerts", alertHandler)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go server.ListenAndServe()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
}

/*
=============================
 HTTP Handler
=============================
*/

func alertHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var payload AlertmanagerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, alert := range payload.Alerts {
		writeJSONLog(alert)
	}

	w.WriteHeader(http.StatusOK)
}

/*
=============================
 JSON Log Writer (Open → Append → Close)
=============================
*/

func writeJSONLog(alert Alert) {
	now := time.Now()

	// Day-wise file name
	fileName := "/var/log/app_hivemq_" + now.Format("20060102") + "0001.log"

	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // fail silently (alert flow must not break)
	}
	defer file.Close()

	hostname := safeHostname(alert.Labels)
	ip := safeIP(alert.Labels)

	entry := JSONLog{
		Timestamp: now.Format("2006-01-02 15:04"),
		IP:        ip,
		Hostname:  hostname,
		KPI:       safeValue(alert.Labels["alertname"], "unknown"),
		Value:     "1",
		Count:     safeValue(alert.Annotations["current_value"], "NA"),
		Summary:   safeValue(alert.Annotations["summary"], "no summary"),
	}

	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(entry)
}

/*
=============================
 Safe Helpers
=============================
*/

func safeHostname(labels map[string]string) string {
	if h, ok := labels["hostname"]; ok && h != "" {
		return h
	}
	if scope, ok := labels["scope"]; ok && scope == "cluster" {
		return "hivemq-cluster"
	}
	return "unknown"
}

func safeIP(labels map[string]string) string {
	instance, ok := labels["instance"]
	if !ok || instance == "" {
		return "NA"
	}

	host, _, err := net.SplitHostPort(instance)
	if err == nil {
		return host
	}

	return strings.Split(instance, ":")[0]
}

func safeValue(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
