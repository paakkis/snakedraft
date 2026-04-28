package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SteamService struct {
	APIKey string
	client *http.Client
}

func NewSteamService(apiKey string) *SteamService {
	return &SteamService{
		APIKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type PlayerSummary struct {
	Steam64ID string `json:"steamid"`
	Name      string `json:"personaname"`
	AvatarURL string `json:"avatarfull"`
}

func (s *SteamService) ResolveVanityURL(vanityURL string) (string, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUser/ResolveVanityURL/v1/?key=%s&vanityurl=%s",
		s.APIKey, vanityURL,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Response struct {
			SteamID string `json:"steamid"`
			Success int    `json:"success"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Response.Success != 1 {
		return "", fmt.Errorf("vanity URL not found")
	}
	return result.Response.SteamID, nil
}

func (s *SteamService) GetPlayerSummary(steam64ID string) (*PlayerSummary, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s",
		s.APIKey, steam64ID,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Response struct {
			Players []PlayerSummary `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Response.Players) == 0 {
		return nil, fmt.Errorf("player not found")
	}
	return &result.Response.Players[0], nil
}

// ParseSteamInput handles three input formats:
//
//	https://steamcommunity.com/id/vanityname     → resolve vanity URL
//	https://steamcommunity.com/profiles/76561... → extract ID directly
//	76561...                                     → use directly
func (s *SteamService) ParseSteamInput(input string) (steam64ID string, vanityURL string, err error) {
	input = strings.TrimSpace(input)

	if strings.Contains(input, "/profiles/") {
		parts := strings.Split(input, "/profiles/")
		steam64ID = strings.Trim(parts[len(parts)-1], "/")
		return steam64ID, "", nil
	}

	if strings.Contains(input, "/id/") {
		parts := strings.Split(input, "/id/")
		vanityURL = strings.Trim(parts[len(parts)-1], "/")
		steam64ID, err = s.ResolveVanityURL(vanityURL)
		return steam64ID, vanityURL, err
	}

	// Assume raw Steam64 ID (17-digit number)
	if len(input) > 10 {
		return input, "", nil
	}

	return "", "", fmt.Errorf("could not parse steam input: %s", input)
}
