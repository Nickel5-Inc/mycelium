package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"
)

// RandomString generates a random string of given length
func RandomString(length int) string {
	bytes := make([]byte, (length+1)/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// RandomStringWithPrefix generates a random string with a prefix
func RandomStringWithPrefix(prefix string, length int) string {
	if len(prefix) >= length {
		return prefix[:length]
	}
	return prefix + RandomString(length-len(prefix))
}

// RandomBytes generates random bytes of given length
func RandomBytes(length int) []byte {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return bytes
}

// RandomInt generates a random integer in range [min, max]
func RandomInt(min, max int) int {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	return int(n.Int64()) + min
}

// RandomInt64 generates a random int64 in range [min, max]
func RandomInt64(min, max int64) int64 {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(max-min+1))
	return n.Int64() + min
}

// RandomFloat64 generates a random float64 in range [min, max]
func RandomFloat64(min, max float64) float64 {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	ratio := float64(n.Int64()) / float64(math.MaxInt64)
	return min + ratio*(max-min)
}

// RandomBool generates a random boolean
func RandomBool() bool {
	n, _ := rand.Int(rand.Reader, big.NewInt(2))
	return n.Int64() == 1
}

// RandomTime generates a random time in range [min, max]
func RandomTime(min, max time.Time) time.Time {
	if min.After(max) {
		min, max = max, min
	}
	if min.Equal(max) {
		return min
	}
	delta := max.Sub(min)
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(delta)))
	return min.Add(time.Duration(n.Int64()))
}

// RandomDuration generates a random duration in range [min, max]
func RandomDuration(min, max time.Duration) time.Duration {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	return min + time.Duration(n.Int64())
}

// RandomChoice returns a random element from slice
func RandomChoice[T any](slice []T) T {
	if len(slice) == 0 {
		panic("empty slice")
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(slice))))
	return slice[n.Int64()]
}

// RandomChoices returns n random elements from slice
func RandomChoices[T any](slice []T, n int) []T {
	if len(slice) == 0 {
		panic("empty slice")
	}
	if n <= 0 {
		return nil
	}
	if n >= len(slice) {
		result := make([]T, len(slice))
		copy(result, slice)
		return result
	}

	indices := make(map[int]bool)
	result := make([]T, 0, n)

	for len(result) < n {
		idx := RandomInt(0, len(slice)-1)
		if !indices[idx] {
			indices[idx] = true
			result = append(result, slice[idx])
		}
	}

	return result
}

// RandomShuffle randomly shuffles a slice
func RandomShuffle[T any](slice []T) {
	n := len(slice)
	for i := n - 1; i > 0; i-- {
		j := RandomInt(0, i)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// RandomIP generates a random IP address
func RandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		RandomInt(0, 255),
		RandomInt(0, 255),
		RandomInt(0, 255),
		RandomInt(0, 255),
	)
}

// RandomPort generates a random port number
func RandomPort() int {
	return RandomInt(1024, 65535)
}

// RandomURL generates a random URL
func RandomURL() string {
	protocols := []string{"http", "https"}
	domains := []string{"example.com", "test.com", "demo.com"}
	paths := []string{"api", "v1", "data", "users", "items"}

	protocol := RandomChoice(protocols)
	domain := RandomChoice(domains)
	numPaths := RandomInt(0, 3)
	pathParts := RandomChoices(paths, numPaths)

	var url strings.Builder
	url.WriteString(protocol)
	url.WriteString("://")
	url.WriteString(domain)
	if len(pathParts) > 0 {
		url.WriteString("/")
		url.WriteString(strings.Join(pathParts, "/"))
	}

	return url.String()
}

// RandomEmail generates a random email address
func RandomEmail() string {
	domains := []string{"example.com", "test.com", "demo.com"}
	domain := RandomChoice(domains)
	username := RandomString(8)
	return fmt.Sprintf("%s@%s", username, domain)
}

// RandomPhoneNumber generates a random phone number
func RandomPhoneNumber() string {
	return fmt.Sprintf("+%d%d%d%d%d%d%d%d%d%d",
		RandomInt(1, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
		RandomInt(0, 9),
	)
}

// RandomUserAgent generates a random user agent string
func RandomUserAgent() string {
	browsers := []string{
		"Chrome", "Firefox", "Safari", "Edge",
	}
	versions := []string{
		"100.0", "99.0", "98.0", "97.0",
	}
	oses := []string{
		"Windows NT 10.0", "Macintosh; Intel Mac OS X 10_15_7", "X11; Linux x86_64",
	}

	browser := RandomChoice(browsers)
	version := RandomChoice(versions)
	os := RandomChoice(oses)

	return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) %s/%s",
		os, browser, version)
}
