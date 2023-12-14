package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/denizumutdereli/stream-services/internal/transport"
)

func CheckEnv(value string, envParam string) string {
	if value == "" {
		log.Fatalf("%s must be set", envParam)
	}
	return value
}

func SplitString(s, delimeter string) []string {

	return strings.Split(s, delimeter)

}

func GetKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func IsMn(r rune) bool {
	return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
}

func RemoveSpacesAndDots(s string) string {

	s = strings.ReplaceAll(s, " ", "")

	s = strings.ReplaceAll(s, ".", "")

	return s
}

func StringPrepareForComparision(s string) string {
	s = strings.ToUpper(s)
	s = strings.TrimSpace(s)
	return s
}

/*
TODO: move to relavent module
*/
func GetAssetPairs(ctx context.Context, redisClient *transport.RedisManager, restClient *transport.Client, redisKey string, path string, newData bool) ([]string, error) {
	var cachedData struct {
		Data []string `json:"data"`
	}

	if !newData {
		err := redisClient.GetKeyValue(ctx, redisKey, &cachedData)
		if err == nil {
			for i, v := range cachedData.Data {
				cachedData.Data[i] = strings.ToUpper(strings.ReplaceAll(v, "-", ""))
			}
			return cachedData.Data, nil
		}

	}

	// Cache does not exist, is malformed, or expired, fetch asset pairs from REST API
	response, err := restClient.DoRequest("GET", path, nil, nil)
	if err != nil {
		return nil, err
	}

	var responseStruct struct {
		Result []struct {
			Name string `json:"name"`
		} `json:"result"`
	}

	if err := json.Unmarshal(response.Body, &responseStruct); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract and normalize pairs
	var pairs []string
	for _, result := range responseStruct.Result {
		normalizedPairName := strings.ToLower(strings.ReplaceAll(result.Name, "-", ""))
		pairs = append(pairs, normalizedPairName)
	}

	err = redisClient.SetKeyValue(ctx, redisKey, map[string][]string{"data": pairs}, time.Hour*24)
	if err != nil {
		return nil, err
	}

	return pairs, nil
}

func IsServiceAllowed(name string, allowedServices []string) bool {
	for _, s := range allowedServices {
		if s == name {
			return true
		}
	}
	return false
}

func Contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func ParseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func NukeMe() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Start()
	if err != nil {
		log.Fatalf("Failed to restart: %s", err)
	}

	os.Exit(0)
}
