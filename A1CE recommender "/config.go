package main

import (
	"os"
)

// Config holds application configuration
type Config struct {
	ServerPort  string
	A1CEBaseURL string
	JWTSecret   string
	UseMockData bool
	LogLevel    string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		ServerPort:  getEnv("PORT", "8080"),
		A1CEBaseURL: getEnv("A1CE_BASE_URL", "https://teaching.cmkl.ai/api"),
		JWTSecret:   getEnv("JWT_SECRET", "your-secret-key"),
		UseMockData: getEnv("USE_MOCK_DATA", "false") == "true",
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

/*
=== SETUP INSTRUCTIONS ===

1. Initialize Go module on the server:
   cd ~/myproject
   go mod init a1ce-recommender

2. Create all the Go files:
   - main.go
   - models.go
   - a1ce_client.go
   - service.go
   - recommender.go
   - optimizer_evaluator.go
   - config.go

3. Install dependencies (if any):
   go mod tidy

4. Build the application:
   go build -o recommender

5. Run the server:
   ./recommender

   Or run directly:
   go run .

6. Test the API:
   curl http://localhost:8080/api/v1/health

7. Test with recommendations (replace JWT token):
   curl -X POST http://localhost:8080/api/v1/recommendations \
     -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{
       "student_id": "S12345",
       "semester": "Spring 2026",
       "max_credit_load": 60
     }'

=== ENVIRONMENT VARIABLES ===

Create a .env file (optional):
PORT=8080
A1CE_BASE_URL=https://teaching.cmkl.ai/api
JWT_SECRET=your-secret-key
USE_MOCK_DATA=false
LOG_LEVEL=info

=== DEPLOYMENT ===

For production deployment:

1. Build for Linux (if developing on Windows):
   GOOS=linux GOARCH=amd64 go build -o recommender

2. Upload to server:
   scp recommender capstone@teaching.cmkl.ai:~/

3. Run as a service using systemd:
   Create /etc/systemd/system/recommender.service:

   [Unit]
   Description=A1CE Course Recommender
   After=network.target

   [Service]
   Type=simple
   User=capstone
   WorkingDirectory=/home/capstone
   ExecStart=/home/capstone/recommender
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target

4. Enable and start service:
   sudo systemctl enable recommender
   sudo systemctl start recommender
   sudo systemctl status recommender

=== NGINX REVERSE PROXY (Optional) ===

If you want to expose the API through NGINX:

server {
    listen 80;
    server_name api.recommender.cmkl.ac.th;

    location /api/v1/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

=== TESTING ===

Create a simple test file (test_api.sh):

#!/bin/bash
# Health check
echo "Testing health endpoint..."
curl http://localhost:8080/api/v1/health

echo -e "\n\nTesting recommendations endpoint..."
curl -X POST http://localhost:8080/api/v1/recommendations \
  -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -d '{
    "student_id": "S12345",
    "semester": "Spring 2026",
    "max_credit_load": 60
  }'

Make it executable:
chmod +x test_api.sh
./test_api.sh

=== TROUBLESHOOTING ===

1. Check if port 8080 is available:
   netstat -tulpn | grep 8080

2. View logs:
   journalctl -u recommender -f

3. Check Go version:
   go version
   (Need Go 1.21 or higher)

4. If A1CE API is unavailable, set USE_MOCK_DATA=true to test with mock data

=== PROJECT STRUCTURE ===

a1ce-recommender/
├── main.go                  # HTTP server & routes
├── models.go                # Data structures
├── a1ce_client.go           # A1CE API integration
├── service.go               # Business logic
├── recommender.go           # Core algorithm
├── optimizer_evaluator.go   # Set optimization & metrics
├── config.go                # Configuration
├── go.mod                   # Go module definition
├── go.sum                   # Dependencies
└── README.md                # Documentation

=== NEXT STEPS ===

1. Implement actual JWT validation with A1CE
2. Add proper error handling and logging
3. Implement caching for course catalog
4. Add rate limiting
5. Create comprehensive tests
6. Add Swagger/OpenAPI documentation
7. Implement monitoring and metrics
8. Add database for storing recommendation history
*/
