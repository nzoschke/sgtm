//go:build !darwin

package main

func runDashboardWebView(url string) error {
	openDashboard(url)
	select {}
}
