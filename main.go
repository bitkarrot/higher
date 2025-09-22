package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitkarrot/higher/keyderivation"
	"github.com/fiatjaf/eventstore/badger"
	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/blossom"
	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/afero"
)

type Config struct {
	RelayName        string
	RelayPubkey      string
	RelayDescription string
	DBEngine         *string
	DBPath           *string
	PostgresUser     *string
	PostgresPassword *string
	PostgresDB       *string
	PostgresHost     *string
	PostgresPort     *string
	TeamDomain       string
	BlossomEnabled   bool
	BlossomPath      *string
	BlossomURL       *string
	WebsocketURL     *string
	AllowedKinds     []int
	MaxUploadSizeMB  int
	// Key derivation / access control
	RelayMnemonic      *string
	RelaySeedHex       *string
	MaxDerivationIndex int
	ReadsRestricted    bool
}

type NostrData struct {
	Names  map[string]string   `json:"names"`
	Relays map[string][]string `json:"relays"`
}

var data NostrData
var relay *khatru.Relay
var db DBBackend
var fs afero.Fs
var config Config
var deriver *keyderivation.NostrKeyDeriver

func main() {
	relay = khatru.NewRelay()
	config = LoadConfig()

	// Initialize key deriver if configured
	if err := initDeriver(config); err != nil {
		log.Fatalf("Failed to initialize key deriver: %v", err)
	}

	// Startup status log
	if deriver != nil {
		log.Printf("Access control: deriver ACTIVE (BIP32), MaxDerivationIndex=%d", config.MaxDerivationIndex)
	} else {
		log.Printf("Access control: deriver INACTIVE")
	}
	if config.ReadsRestricted {
		log.Printf("Reads restriction: ENABLED (queries must specify authors derived from master)")
	} else {
		log.Printf("Reads restriction: DISABLED")
	}

	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.QueryEvents = append(relay.QueryEvents, db.QueryEvents)

	if config.TeamDomain != "" {
		fetchNostrData(config.TeamDomain)

		go func() {
			for {
				time.Sleep(1 * time.Hour)
				fetchNostrData(config.TeamDomain)
			}
		}()
	}

	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		// If we have a deriver and the event pubkey belongs to master, allow writes (subject to allowed kinds)
		belongsToMaster := false
		if deriver != nil {
			b, _, err := deriver.CheckKeyBelongsToMaster(event.PubKey, uint32(config.MaxDerivationIndex), true)
			if err != nil {
				log.Printf("Error checking key against master: %v", err)
			}
			belongsToMaster = b
		}
		// If TEAM_DOMAIN is set and the key does NOT belong to master, enforce team membership; otherwise, skip this check
		if config.TeamDomain != "" && !belongsToMaster {
			// Check if user is part of the team
			isTeamMember := false
			for _, pubkey := range data.Names {
				if event.PubKey == pubkey {
					isTeamMember = true
					break
				}
			}
			if !isTeamMember {
				return true, "you are not part of the team"
			}
		}

		// Check if event kind is allowed
		if len(config.AllowedKinds) > 0 {
			isKindAllowed := false
			for _, allowedKind := range config.AllowedKinds {
				if event.Kind == allowedKind {
					isKindAllowed = true
					break
				}
			}
			if !isKindAllowed {
				return true, fmt.Sprintf("event kind %d is not allowed", event.Kind)
			}
		}

		return false, "" // allow
	})

	// Optionally restrict reads: only allow filters that target authors derived from master
	if config.ReadsRestricted {
		relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
			if deriver == nil {
				// If we cannot validate, reject by default when reads are restricted
				return true, "reads are restricted but key deriver is not configured"
			}
			// If authors are provided, ensure all are descendants of master
			if len(filter.Authors) > 0 {
				for _, a := range filter.Authors {
					belongs, _, err := deriver.CheckKeyBelongsToMaster(a, uint32(config.MaxDerivationIndex), true)
					if err != nil {
						return true, fmt.Sprintf("error validating author: %v", err)
					}
					if !belongs {
						return true, "author not allowed by read restrictions"
					}
				}
				return false, ""
			}
			// If no authors specified, disallow broad reads under restriction
			return true, "reads restricted: specify allowed authors"
		})
	}

	// Setup front page handler
	setupFrontPageHandler(relay, config)

	// Add handler for TeamHigher.jpg
	relay.Router().HandleFunc("/public/TeamHigher.jpg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./public/TeamHigher.jpg")
	})

	if !config.BlossomEnabled {
		// Configure HTTP server with timeouts suitable for large file uploads
		server := &http.Server{
			Addr:              ":3334",
			Handler:           relay,
			ReadTimeout:       15 * time.Minute, // Increased to 15 minutes for very large files
			WriteTimeout:      15 * time.Minute, // Increased to 15 minutes
			IdleTimeout:       5 * time.Minute,  // Increased idle timeout
			ReadHeaderTimeout: 30 * time.Second, // Prevent slow header attacks
			MaxHeaderBytes:    1 << 20,          // 1MB max header size
		}

		fmt.Println("running on :3334 with extended timeouts for large uploads")
		server.ListenAndServe()
		return
	}

	bl := blossom.New(relay, *config.BlossomURL)
	bl.Store = blossom.EventStoreBlobIndexWrapper{Store: db, ServiceURL: bl.ServiceURL}
	bl.StoreBlob = append(bl.StoreBlob, func(ctx context.Context, sha256 string, body []byte) error {
		// Create context with timeout for large file operations
		storeCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		file, err := fs.Create(*config.BlossomPath + sha256)
		if err != nil {
			return err
		}
		defer file.Close()

		// Use streaming copy with context checking for large files
		reader := bytes.NewReader(body)
		buffer := make([]byte, 32*1024) // 32KB buffer for efficient copying

		for {
			select {
			case <-storeCtx.Done():
				return storeCtx.Err()
			default:
			}

			n, err := reader.Read(buffer)
			if n > 0 {
				if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
					return writeErr
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}

		return file.Sync() // Ensure data is written to disk
	})

	bl.LoadBlob = append(bl.LoadBlob, func(ctx context.Context, sha256 string) (io.ReadSeeker, error) {
		filePath := *config.BlossomPath + sha256
		log.Printf("LoadBlob: Attempting to open file at path: %s", filePath)
		file, err := fs.Open(filePath)
		if err != nil {
			log.Printf("LoadBlob: Failed to open file %s: %v", filePath, err)
			return nil, err
		}
		log.Printf("LoadBlob: Successfully opened file %s", filePath)
		return file, nil
	})
	bl.DeleteBlob = append(bl.DeleteBlob, func(ctx context.Context, sha256 string) error {
		return fs.Remove(*config.BlossomPath + sha256)
	})
	bl.RejectUpload = append(bl.RejectUpload, func(ctx context.Context, event *nostr.Event, size int, ext string) (bool, string, int) {
		// Check for configurable size limit
		maxSize := config.MaxUploadSizeMB * 1024 * 1024
		if size > maxSize {
			return true, fmt.Sprintf("file size exceeds %dMB limit", config.MaxUploadSizeMB), 413
		}

		// If TEAM_DOMAIN is set, enforce team membership; otherwise, skip this check
		if config.TeamDomain != "" {
			for _, pubkey := range data.Names {
				if pubkey == event.PubKey {
					return false, ext, size
				}
			}

			return true, "you are not part of the team", 403
		}

		// TEAM_DOMAIN is not set, allow upload (size already checked)
		return false, ext, size
	})

	// Add custom list endpoint for Sakura health checks
	relay.Router().HandleFunc("/list/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract pubkey from URL path
		pubkey := strings.TrimPrefix(r.URL.Path, "/list/")
		if pubkey == "" {
			http.Error(w, "Missing pubkey", http.StatusBadRequest)
			return
		}

		log.Printf("List blobs request for pubkey: %s", pubkey)

		// Read all files from the blossom directory
		blobs := []map[string]interface{}{}

		if config.BlossomPath != nil {
			file, err := fs.Open(*config.BlossomPath)
			if err != nil {
				log.Printf("Error opening blossom directory: %v", err)
			} else {
				defer file.Close()
				fileInfos, err := file.Readdir(-1)
				if err != nil {
					log.Printf("Error reading blossom directory: %v", err)
				} else {
					for _, fileInfo := range fileInfos {
						if !fileInfo.IsDir() {
							fileName := fileInfo.Name()
							// Validate that it looks like a SHA256 hash (64 hex characters)
							if len(fileName) == 64 {
								isValidHash := true
								for _, char := range fileName {
									if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
										isValidHash = false
										break
									}
								}

								if isValidHash {
									// Detect MIME type by reading the first 512 bytes
									contentType := "application/octet-stream" // Default fallback
									filePath := *config.BlossomPath + fileName
									if blobFile, err := fs.Open(filePath); err == nil {
										buffer := make([]byte, 512)
										if n, err := blobFile.Read(buffer); err == nil && n > 0 {
											detectedType := http.DetectContentType(buffer[:n])
											if detectedType != "" {
												contentType = detectedType
											}
										}
										blobFile.Close()
									}

									blob := map[string]interface{}{
										"sha256":   strings.ToLower(fileName),
										"size":     fileInfo.Size(),
										"type":     contentType,
										"url":      *config.BlossomURL + "/" + strings.ToLower(fileName),
										"uploaded": fileInfo.ModTime().Unix(),
									}
									blobs = append(blobs, blob)
									log.Printf("Found blob: %s (size: %d, type: %s)", fileName, fileInfo.Size(), contentType)
								}
							}
						}
					}
				}
			}
		}

		log.Printf("Returning %d blobs for pubkey %s", len(blobs), pubkey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(blobs)
	})

	// Add custom mirror endpoint handler for Sakura compatibility
	relay.Router().HandleFunc("/mirror", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the request body to get source URL
		var mirrorRequest struct {
			URL string `json:"url"`
		}

		if err := json.NewDecoder(r.Body).Decode(&mirrorRequest); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if mirrorRequest.URL == "" {
			http.Error(w, "Missing source URL", http.StatusBadRequest)
			return
		}

		// Extract blob hash from source URL
		blobHash := extractSha256FromURL(mirrorRequest.URL)
		if blobHash == "" {
			http.Error(w, "Cannot extract blob hash from source URL", http.StatusBadRequest)
			return
		}

		// Check if blob already exists
		if _, err := fs.Open(*config.BlossomPath + blobHash); err == nil {
			// Blob already exists, return success
			response := map[string]interface{}{
				"sha256": blobHash,
				"url":    *config.BlossomURL + "/" + blobHash,
				"size":   0, // We don't know the size without reading the file
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		// Download blob from source URL
		resp, err := http.Get(mirrorRequest.URL)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to fetch source blob: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("Source server returned %d", resp.StatusCode), http.StatusBadGateway)
			return
		}

		// Read and verify the blob content
		blobData, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read blob data: %v", err), http.StatusInternalServerError)
			return
		}

		// Verify the hash matches
		hasher := sha256.New()
		hasher.Write(blobData)
		actualHash := hex.EncodeToString(hasher.Sum(nil))

		if actualHash != blobHash {
			http.Error(w, "Blob hash mismatch", http.StatusBadRequest)
			return
		}

		// Store the blob using the existing StoreBlob functionality
		ctx := r.Context()
		for _, storeFunc := range bl.StoreBlob {
			if err := storeFunc(ctx, blobHash, blobData); err != nil {
				http.Error(w, fmt.Sprintf("Failed to store blob: %v", err), http.StatusInternalServerError)
				return
			}
		}

		// Return success response
		response := map[string]interface{}{
			"sha256": blobHash,
			"url":    *config.BlossomURL + "/" + blobHash,
			"size":   len(blobData),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

		log.Printf("Successfully mirrored blob %s from %s", blobHash, mirrorRequest.URL)
	})

	// Configure HTTP server with timeouts suitable for large file uploads
	server := &http.Server{
		Addr:              ":3334",
		Handler:           relay,
		ReadTimeout:       15 * time.Minute, // Increased to 15 minutes for very large files
		WriteTimeout:      15 * time.Minute, // Increased to 15 minutes
		IdleTimeout:       5 * time.Minute,  // Increased idle timeout
		ReadHeaderTimeout: 30 * time.Second, // Prevent slow header attacks
		MaxHeaderBytes:    1 << 20,          // 1MB max header size
	}

	fmt.Println("running on :3334 with extended timeouts for large uploads")
	server.ListenAndServe()
}

func fetchNostrData(teamDomain string) {
	if teamDomain == "" {
		log.Println("TEAM_DOMAIN not set; skipping Nostr data fetch")
		return
	}
	response, err := http.Get("https://" + teamDomain + "/.well-known/nostr.json")
	if err != nil {
		log.Printf("Error getting well known file: %v", err)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}

	var newData NostrData
	err = json.Unmarshal(body, &newData)
	if err != nil {
		log.Printf("Error unmarshalling JSON: %v", err)
		return
	}

	data = newData
	for pubkey, names := range data.Names {
		fmt.Println(pubkey, names)
	}

	log.Println("Updated NostrData from .well-known file")
}

func LoadConfig() Config {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	config := Config{
		RelayName:          getEnv("RELAY_NAME"),
		RelayPubkey:        getEnv("RELAY_PUBKEY"),
		RelayDescription:   getEnv("RELAY_DESCRIPTION"),
		DBEngine:           getEnvNullable("DB_ENGINE"),
		DBPath:             getEnvNullable("DB_PATH"),
		PostgresUser:       getEnvNullable("POSTGRES_USER"),
		PostgresPassword:   getEnvNullable("POSTGRES_PASSWORD"),
		PostgresDB:         getEnvNullable("POSTGRES_DB"),
		PostgresHost:       getEnvNullable("POSTGRES_HOST"),
		PostgresPort:       getEnvNullable("POSTGRES_PORT"),
		TeamDomain:         getEnvWithDefault("TEAM_DOMAIN", ""),
		BlossomEnabled:     getEnvBool("BLOSSOM_ENABLED"),
		BlossomPath:        getEnvNullable("BLOSSOM_PATH"),
		BlossomURL:         getEnvNullable("BLOSSOM_URL"),
		WebsocketURL:       getEnvNullable("WEBSOCKET_URL"),
		AllowedKinds:       parseAllowedKinds(getEnvNullable("ALLOWED_KINDS")),
		MaxUploadSizeMB:    getEnvIntWithDefault("MAX_UPLOAD_SIZE_MB", 200),
		RelayMnemonic:      getEnvNullable("RELAY_MNEMONIC"),
		RelaySeedHex:       getEnvNullable("RELAY_SEED_HEX"),
		MaxDerivationIndex: getEnvIntWithDefault("MAX_DERIVATION_INDEX", 100),
		ReadsRestricted:    getEnvBool("READS_RESTRICTED"),
	}

	// Enforce exactly one of RELAY_MNEMONIC or RELAY_SEED_HEX must be set
	hasMnemonic := config.RelayMnemonic != nil && strings.TrimSpace(*config.RelayMnemonic) != ""
	hasSeed := config.RelaySeedHex != nil && strings.TrimSpace(*config.RelaySeedHex) != ""
	if hasMnemonic == hasSeed { // either both true or both false
		log.Fatalf("Configuration error: you must set exactly one of RELAY_MNEMONIC or RELAY_SEED_HEX")
	}

	relay.Info.Name = config.RelayName
	relay.Info.PubKey = config.RelayPubkey
	relay.Info.Description = config.RelayDescription
	if config.DBPath == nil {
		defaultPath := "db/"
		config.DBPath = &defaultPath
	}

	db = newDBBackend(*config.DBPath)

	if err := db.Init(); err != nil {
		panic(err)
	}

	fs = afero.NewOsFs()
	if config.BlossomEnabled {
		if config.BlossomPath == nil {
			log.Fatalf("Blossom enabled but no path set")
		}
		fs.MkdirAll(*config.BlossomPath, 0755)
	}

	return config
}

func initDeriver(cfg Config) error {
	// Initialize the global deriver based on mnemonic or seed hex
	// Exactly one of these should be set by LoadConfig() validation
	if cfg.RelayMnemonic != nil && strings.TrimSpace(*cfg.RelayMnemonic) != "" {
		d, err := keyderivation.NewNostrKeyDeriver(strings.TrimSpace(*cfg.RelayMnemonic))
		if err != nil {
			return fmt.Errorf("failed to create deriver from mnemonic: %w", err)
		}
		deriver = d
		return nil
	}

	if cfg.RelaySeedHex != nil && strings.TrimSpace(*cfg.RelaySeedHex) != "" {
		seedBytes, err := hex.DecodeString(strings.TrimSpace(*cfg.RelaySeedHex))
		if err != nil {
			return fmt.Errorf("invalid RELAY_SEED_HEX: %w", err)
		}
		d, err := keyderivation.NewNostrKeyDeriverFromSeed(seedBytes)
		if err != nil {
			return fmt.Errorf("failed to create deriver from seed: %w", err)
		}
		deriver = d
		return nil
	}

	// Neither provided: leave deriver nil (should not happen due to LoadConfig fatal)
	deriver = nil
	return nil
}

func getEnv(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		log.Fatalf("Environment variable %s not set", key)
	}
	return value
}

func getEnvBool(key string) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return false
	}
	return value == "true"
}

func getEnvNullable(key string) *string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return nil
	}
	return &value
}

func getEnvIntWithDefault(key string, defaultValue int) int {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Warning: Invalid integer value '%s' for %s, using default %d", value, key, defaultValue)
		return defaultValue
	}
	return intValue
}

func getEnvWithDefault(key string, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func parseAllowedKinds(allowedKindsStr *string) []int {
	if allowedKindsStr == nil || strings.TrimSpace(*allowedKindsStr) == "" {
		return []int{} // Empty slice means allow all kinds
	}

	kindsStr := strings.TrimSpace(*allowedKindsStr)
	kindStrings := strings.Split(kindsStr, ",")
	var kinds []int

	for _, kindStr := range kindStrings {
		kindStr = strings.TrimSpace(kindStr)
		if kindStr == "" {
			continue
		}

		kind, err := strconv.Atoi(kindStr)
		if err != nil {
			log.Printf("Warning: Invalid kind '%s' in ALLOWED_KINDS, skipping", kindStr)
			continue
		}
		kinds = append(kinds, kind)
	}

	if len(kinds) > 0 {
		log.Printf("Relay configured to only allow kinds: %v", kinds)
	} else {
		log.Printf("Relay configured to allow all kinds")
	}

	return kinds
}

type DBBackend interface {
	Init() error
	Close()
	CountEvents(ctx context.Context, filter nostr.Filter) (int64, error)
	DeleteEvent(ctx context.Context, evt *nostr.Event) error
	QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error)
	SaveEvent(ctx context.Context, evt *nostr.Event) error
	ReplaceEvent(ctx context.Context, evt *nostr.Event) error
}

func newDBBackend(path string) DBBackend {
	// Default to Badger if DB_ENGINE is not set or empty
	if config.DBEngine == nil || strings.TrimSpace(*config.DBEngine) == "" {
		defaultEngine := "badger"
		config.DBEngine = &defaultEngine
	}

	// Log chosen engine for clarity
	log.Printf("DB engine selected: %s", *config.DBEngine)

	switch strings.ToLower(strings.TrimSpace(*config.DBEngine)) {
	case "lmdb":
		return newLMDBBackend(path)
	case "postgres":
		return newPostgresBackend()
	case "badger":
		return &badger.BadgerBackend{Path: path}
	default:
		// Fallback to Badger for any unknown value
		log.Printf("Unknown DB_ENGINE '%s', defaulting to badger", *config.DBEngine)
		return &badger.BadgerBackend{Path: path}
	}
}

func newPostgresBackend() DBBackend {
	// Validate required Postgres settings to avoid nil pointer panics
	if config.PostgresUser == nil || strings.TrimSpace(*config.PostgresUser) == "" ||
		config.PostgresPassword == nil || strings.TrimSpace(*config.PostgresPassword) == "" ||
		config.PostgresDB == nil || strings.TrimSpace(*config.PostgresDB) == "" ||
		config.PostgresHost == nil || strings.TrimSpace(*config.PostgresHost) == "" ||
		config.PostgresPort == nil || strings.TrimSpace(*config.PostgresPort) == "" {
		log.Fatalf("Postgres selected but configuration is incomplete: ensure POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, POSTGRES_HOST, POSTGRES_PORT are set")
	}

	return &postgresql.PostgresBackend{
		DatabaseURL: fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			*config.PostgresUser, *config.PostgresPassword, *config.PostgresHost, *config.PostgresPort, *config.PostgresDB),
	}
}

// extractSha256FromURL extracts the SHA256 hash from a blossom URL
// Expected format: https://server.com/sha256hash or https://server.com/sha256hash.ext
func extractSha256FromURL(url string) string {
	// Remove the protocol and domain
	parts := strings.Split(url, "/")
	if len(parts) < 4 {
		return ""
	}

	// Get the last part which should be the hash (possibly with extension)
	hashPart := parts[len(parts)-1]

	// Remove file extension if present
	if dotIndex := strings.LastIndex(hashPart, "."); dotIndex != -1 {
		hashPart = hashPart[:dotIndex]
	}

	// Validate that it looks like a SHA256 hash (64 hex characters)
	if len(hashPart) == 64 {
		// Check if all characters are valid hex
		for _, char := range hashPart {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return ""
			}
		}
		return strings.ToLower(hashPart)
	}

	return ""
}
