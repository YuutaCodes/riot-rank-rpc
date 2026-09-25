package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
)

var localClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

type EntitlementsToken struct {
	AccessToken string
	Token       string
	Subject     string
	Issuer      string
}

func fetchEntitlements(lf Lockfile) (EntitlementsToken, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/entitlements/v1/token", lf.Port)
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return EntitlementsToken{}, err
	}

	req.SetBasicAuth("riot", lf.Password)

	resp, err := localClient.Do(req)
	if err != nil {
		return EntitlementsToken{}, err
	}

	defer resp.Body.Close()

	var result EntitlementsToken
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return EntitlementsToken{}, err
	}

	return result, nil
}

type RegionLocale struct {
}
