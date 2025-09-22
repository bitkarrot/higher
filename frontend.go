package main

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/fiatjaf/khatru"
)

const frontPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.RelayName}} - Nostr Relay & Blossom Server</title>
    
    <!-- Open Graph / Link Preview Meta Tags -->
    <meta property="og:type" content="website">
    <meta property="og:title" content="{{.RelayName}} - Nostr Relay & Blossom Server">
    <meta property="og:description" content="{{.RelayDescription}} - Team-based Nostr relay with Blossom file storage">
    <meta property="og:image" content="https://higher.bitkarrot.co/public/TeamHigher.jpg">
    <meta property="og:url" content="https://{{.TeamDomain}}">
    
    <!-- Twitter Card Meta Tags -->
    <meta name="twitter:card" content="summary">
    <meta name="twitter:title" content="{{.RelayName}} - Nostr Relay & Blossom Server">
    <meta name="twitter:description" content="{{.RelayDescription}} - Team-based Nostr relay with Blossom file storage">
    <meta name="twitter:image" content="https://higher.bitkarrot.co/public/TeamHigher.jpg">
    
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            line-height: 1.6;
            color: #e5e7eb;
            background: linear-gradient(135deg, #0f172a 0%, #1f2937 100%);
            min-height: 100vh;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 2rem;
        }
        
        .header {
            text-align: center;
            color: white;
            margin-bottom: 3rem;
        }
        
        .header-content {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 1rem;
        }
        
        .header-logo {
            width: 100px;
            height: 100px;
            object-fit: contain;
        }
        
        .header h1 {
            font-size: 3rem;
            margin-bottom: 0.5rem;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        
        .header p {
            font-size: 1.2rem;
            opacity: 0.9;
        }
        
        .card {
            background: #1f2937;
            border-radius: 12px;
            padding: 2rem;
            margin-bottom: 2rem;
            box-shadow: 0 10px 30px rgba(0,0,0,0.4);
            transition: transform 0.3s ease;
        }
        
        .card:hover {
            transform: translateY(-5px);
        }
        
        .card h2 {
            color: #e5e7eb;
            margin-bottom: 1rem;
            font-size: 1.5rem;
            border-bottom: 2px solid #374151;
            padding-bottom: 0.5rem;
        }
        
        .endpoint {
            margin-bottom: 1.5rem;
            padding: 1rem;
            background: #111827;
            border-radius: 8px;
            border-left: 4px solid #3b82f6;
        }
        
        .endpoint-title {
            font-weight: bold;
            color: #e5e7eb;
            margin-bottom: 0.5rem;
        }
        
        .method {
            display: inline-block;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.8rem;
            font-weight: bold;
            margin-right: 0.5rem;
        }
        
        .method.get { background: #48bb78; color: white; }
        .method.post { background: #ed8936; color: white; }
        .method.put { background: #4299e1; color: white; }
        .method.websocket { background: #805ad5; color: white; }
        
        .path {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            background: #2d3748;
            color: #e2e8f0;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.9rem;
        }
        
        .description {
            color: #cbd5e1;
            margin-top: 0.5rem;
        }
        
        .status-info {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-top: 1rem;
        }
        
        .status-item {
            background: #111827;
            padding: 1rem;
            border-radius: 8px;
            text-align: center;
        }
        
        .status-label {
            font-size: 0.8rem;
            color: #94a3b8;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        
        .status-value {
            font-size: 1.2rem;
            font-weight: bold;
            color: #e5e7eb;
            margin-top: 0.25rem;
        }
        
        .footer {
            text-align: center;
            color: white;
            opacity: 0.8;
            margin-top: 3rem;
        }
        
        .footer a {
            color: white;
            text-decoration: none;
            border-bottom: 1px solid rgba(255,255,255,0.3);
        }
        
        .footer a:hover {
            border-bottom-color: white;
        }
        
        @media (max-width: 768px) {
            .container {
                padding: 1rem;
            }
            
            .header h1 {
                font-size: 2rem;
            }
            
            .card {
                padding: 1.5rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="header-content">
                <img src="/public/TeamHigher.jpg" alt="TeamHive Logo" class="header-logo">
                <h1>{{.RelayName}}</h1>
            </div>
            <p>{{.RelayDescription}}</p>
        </div>
        
        <div class="card">
            <h2>🔗 Nostr Relay Endpoints</h2>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method websocket">WebSocket</span>
                    <span class="path">{{.WebSocketURL}}</span>
                </div>
                <div class="description">
                    Main Nostr relay WebSocket endpoint for publishing and subscribing to events.
                    Supports standard Nostr protocol (NIP-01) with team-based access control.
                </div>
            </div>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method get">GET</span>
                    <span class="path">{{.WellKnownURL}}</span>
                </div>
                <div class="description">
                    Nostr relay information document (NIP-11) containing relay metadata and policies.
                </div>
            </div>
        </div>
        
        {{if .BlossomEnabled}}
        <div class="card">
            <h2>🌸 Blossom Server Endpoints</h2>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method get">GET</span>
                    <span class="path">/{sha256}</span>
                </div>
                <div class="description">
                    Download a blob by its SHA256 hash. Returns the raw file content with appropriate MIME type.
                </div>
            </div>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method put">PUT</span>
                    <span class="path">/upload</span>
                </div>
                <div class="description">
                    Upload a new blob to the server. Requires Nostr event authentication (NIP-98).
                    Maximum file size: {{.MaxUploadSizeMB}}MB.
                </div>
            </div>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method get">GET</span>
                    <span class="path">/list/{pubkey}</span>
                </div>
                <div class="description">
                    List all blobs with metadata including SHA256, size, MIME type, and upload timestamp.
                    Used by Sakura for health checks and blob discovery.
                </div>
            </div>
            
            <div class="endpoint">
                <div class="endpoint-title">
                    <span class="method put">PUT</span>
                    <span class="path">/mirror</span>
                </div>
                <div class="description">
                    Mirror a blob from another Blossom server. Accepts JSON body with source URL,
                    downloads and verifies the blob, then stores it locally.
                </div>
            </div>
        </div>
        {{end}}
        
        <div class="card">
            <h2>📊 Server Status</h2>
            <div class="status-info">
                <div class="status-item">
                    <div class="status-label">Team Domain</div>
                    <div class="status-value">{{.TeamDomain}}</div>
                </div>
                {{if .BlossomEnabled}}
                <div class="status-item">
                    <div class="status-label">Blossom URL</div>
                    <div class="status-value">{{.BlossomURL}}</div>
                </div>
                <div class="status-item">
                    <div class="status-label">Max Upload Size</div>
                    <div class="status-value">{{.MaxUploadSizeMB}}MB</div>
                </div>
                {{end}}
                <div class="status-item">
                    <div class="status-label">Access Control</div>
                    <div class="status-value">Team Members Only</div>
                </div>
                {{if .AllowedKindsStr}}
                <div class="status-item">
                    <div class="status-label">Allowed Event Kinds</div>
                    <div class="status-value">{{.AllowedKindsStr}}</div>
                </div>
                {{end}}
            </div>
        </div>
        
        <div class="footer">
            <p>
                Powered by <a href="https://khatru.nostr.technology/" target="_blank">Khatru</a> 
                {{if .BlossomEnabled}}& <a href="https://khatru.nostr.technology/core/blossom" target="_blank">Blossom</a>{{end}}
                | Built for the Nostr ecosystem
            </p>
        </div>
    </div>
</body>
</html>`

type FrontPageData struct {
	RelayName        string
	RelayDescription string
	TeamDomain       string
	BlossomEnabled   bool
	BlossomURL       string
	MaxUploadSizeMB  int
	AllowedKindsStr  string
	WebSocketURL     string
	WellKnownURL     string
}

func setupFrontPageHandler(relay *khatru.Relay, config Config) {
	relay.Router().HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only serve the front page for GET requests to the root path
		if r.Method != "GET" || r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// Check if this is a WebSocket upgrade request
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			// Let the relay handle WebSocket connections
			relay.ServeHTTP(w, r)
			return
		}

		// Prepare template data
		// Compute WebSocket URL: use config.WebsocketURL if set, otherwise default to wss://{TeamDomain}
		wsURL := "wss://" + config.TeamDomain
		if config.WebsocketURL != nil && strings.TrimSpace(*config.WebsocketURL) != "" {
			wsURL = *config.WebsocketURL
		}

		data := FrontPageData{
			RelayName:        config.RelayName,
			RelayDescription: config.RelayDescription,
			TeamDomain:       config.TeamDomain,
			BlossomEnabled:   config.BlossomEnabled,
			MaxUploadSizeMB:  config.MaxUploadSizeMB,
			WebSocketURL:     wsURL,
			WellKnownURL:     "https://" + config.TeamDomain + "/.well-known/nostr.json",
		}

		if config.BlossomURL != nil {
			data.BlossomURL = *config.BlossomURL
		}

		// Format allowed kinds for display
		if len(config.AllowedKinds) > 0 {
			kindStrs := make([]string, len(config.AllowedKinds))
			for i, kind := range config.AllowedKinds {
				kindStrs[i] = strconv.Itoa(kind)
			}
			data.AllowedKindsStr = strings.Join(kindStrs, ", ")
		}

		// Parse and execute template
		tmpl, err := template.New("frontpage").Parse(frontPageTemplate)
		if err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Template execution error", http.StatusInternalServerError)
			return
		}
	})
}
