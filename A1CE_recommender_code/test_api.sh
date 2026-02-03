#!/bin/bash

# A1CE Recommender API Test Script
# Usage: ./test_api.sh

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_BASE_URL="http://localhost:8080/api/v1"
JWT_TOKEN="" # Leave empty for testing without auth

# Function to print colored output
print_header() {
    echo -e "${BLUE}================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# Function to make API call and format output
api_call() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    print_header "$description"
    echo -e "${YELLOW}Request:${NC} $method $endpoint"
    
    if [ -n "$data" ]; then
        echo -e "${YELLOW}Body:${NC}"
        echo "$data" | jq '.' 2>/dev/null || echo "$data"
    fi
    
    echo ""
    echo -e "${YELLOW}Response:${NC}"
    
    # Build curl command
    local curl_cmd="curl -s -w '\nHTTP_CODE:%{http_code}'"
    
    if [ -n "$JWT_TOKEN" ]; then
        curl_cmd="$curl_cmd -H 'Authorization: Bearer $JWT_TOKEN'"
    fi
    
    if [ "$method" = "POST" ]; then
        curl_cmd="$curl_cmd -X POST -H 'Content-Type: application/json' -d '$data'"
    fi
    
    curl_cmd="$curl_cmd '$API_BASE_URL$endpoint'"
    
    # Execute curl and capture response
    local response=$(eval $curl_cmd)
    local http_code=$(echo "$response" | grep -o 'HTTP_CODE:[0-9]*' | cut -d':' -f2)
    local body=$(echo "$response" | sed 's/HTTP_CODE:[0-9]*$//')
    
    # Display formatted response
    echo "$body" | jq '.' 2>/dev/null || echo "$body"
    
    echo ""
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "Status: $http_code (Success)"
    elif [ "$http_code" -ge 400 ]; then
        print_error "Status: $http_code (Error)"
    else
        print_info "Status: $http_code"
    fi
    
    echo ""
    echo ""
}

# Check if server is running
check_server() {
    print_header "Checking if server is running"
    if curl -s "$API_BASE_URL/health" > /dev/null 2>&1; then
        print_success "Server is running at $API_BASE_URL"
    else
        print_error "Cannot connect to server at $API_BASE_URL"
        print_info "Make sure your Go server is running: go run ."
        exit 1
    fi
    echo ""
}

# Test 1: Health Check
test_health() {
    api_call "GET" "/health" "" "Test 1: Health Check"
}

# Test 2: Get Recommendations
test_recommendations() {
    local data='{
  "student_id": "S12345",
  "semester": "Spring 2026",
  "max_credit_load": 60,
  "max_sets": 1
}'
    api_call "POST" "/recommendations" "$data" "Test 2: Generate Course Recommendations"
}

# Test 3: Get Recommendations with Constraints
test_recommendations_with_constraints() {
    local data='{
  "student_id": "S12345",
  "semester": "Spring 2026",
  "max_credit_load": 48,
  "constraints": {
    "preferred_subdomains": ["AI", "Software Engineering"],
    "exclude_courses": ["CS999"]
  }
}'
    api_call "POST" "/recommendations" "$data" "Test 3: Recommendations with Constraints"
}

# Test 4: Get Student Data
test_student_data() {
    api_call "GET" "/student-data?student_id=S12345" "" "Test 4: Get Student Data"
}

# Test 5: Get Course Catalog
test_course_catalog() {
    api_call "GET" "/course-catalog?semester=Spring%202026&curriculum_version=2024" "" "Test 5: Get Course Catalog"
}

# Test 6: Error - Missing Student ID
test_error_missing_student_id() {
    local data='{
  "semester": "Spring 2026",
  "max_credit_load": 60
}'
    api_call "POST" "/recommendations" "$data" "Test 6: Error Test - Missing Student ID"
}

# Test 7: Error - Invalid Endpoint
test_error_invalid_endpoint() {
    api_call "GET" "/invalid-endpoint" "" "Test 7: Error Test - Invalid Endpoint"
}

# Main menu
show_menu() {
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║  A1CE Recommender API Test Suite      ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
    echo "Select a test to run:"
    echo "  1. Health Check"
    echo "  2. Generate Recommendations"
    echo "  3. Recommendations with Constraints"
    echo "  4. Get Student Data"
    echo "  5. Get Course Catalog"
    echo "  6. Error Test - Missing Required Field"
    echo "  7. Error Test - Invalid Endpoint"
    echo "  8. Run All Tests"
    echo "  9. Check Server Status"
    echo "  0. Exit"
    echo ""
    echo -n "Enter choice [0-9]: "
}

# Run all tests
run_all_tests() {
    print_header "Running All Tests"
    echo ""
    
    test_health
    sleep 1
    
    test_recommendations
    sleep 1
    
    test_recommendations_with_constraints
    sleep 1
    
    test_student_data
    sleep 1
    
    test_course_catalog
    sleep 1
    
    test_error_missing_student_id
    sleep 1
    
    test_error_invalid_endpoint
    
    print_header "All Tests Completed"
}

# Check for jq installation
if ! command -v jq &> /dev/null; then
    print_info "jq is not installed. Responses will not be formatted."
    print_info "To install jq: sudo apt-get install jq (Ubuntu/Debian) or brew install jq (Mac)"
    echo ""
fi

# Check server before showing menu
check_server

# Main loop
while true; do
    show_menu
    read choice
    echo ""
    
    case $choice in
        1) test_health ;;
        2) test_recommendations ;;
        3) test_recommendations_with_constraints ;;
        4) test_student_data ;;
        5) test_course_catalog ;;
        6) test_error_missing_student_id ;;
        7) test_error_invalid_endpoint ;;
        8) run_all_tests ;;
        9) check_server ;;
        0) 
            echo "Goodbye!"
            exit 0
            ;;
        *)
            print_error "Invalid choice. Please select 0-9."
            echo ""
            ;;
    esac
    
    echo -e "${YELLOW}Press Enter to continue...${NC}"
    read
    clear
done