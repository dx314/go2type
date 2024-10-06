package main

import (
	"encoding/json"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

var Version = "v0.9.17"

func printVersion() {
	fmt.Printf("go2type version %s\n", Version)
	if info, ok := debug.ReadBuildInfo(); ok {
		fmt.Printf("go version: %s\n", info.GoVersion)
	}

	latestVersion, err := getLatestVersion()
	if err != nil {
		fmt.Printf("Failed to check for updates: %v\n", err)
		return
	}

	currentVer, err := semver.NewVersion(strings.TrimPrefix(Version, "v"))
	if err != nil {
		fmt.Printf("Error parsing current version: %v\n", err)
		return
	}

	latestVer, err := semver.NewVersion(strings.TrimPrefix(latestVersion, "v"))
	if err != nil {
		fmt.Printf("Error parsing latest version: %v\n", err)
		return
	}

	if latestVer.GreaterThan(currentVer) {
		fmt.Printf("A new version is available: %s\n", latestVersion)
		fmt.Println("You can update by running: go install github.com/dx314/go2type@" + latestVersion)
	} else {
		fmt.Println("You are using the latest version.")
	}
}

func getLatestVersion() (string, error) {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/dx314/go2type/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}
