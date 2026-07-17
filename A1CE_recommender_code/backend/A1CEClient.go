package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CachedDetail holds the result of one getCompetencyDetail call so the optimizer
// can read from memory instead of making HTTP calls during packaging.
type CachedDetail struct {
	StartDate      string
	EndDate        string
	IsAssessmentOnly bool
	IsRequired     bool
	Schedule       []CourseSchedule
}

type A1CEClient struct {
	BaseURL          string
	HTTPClient       *http.Client
	JWTToken         string
	TokenExpiresAt   time.Time
	UniversityCode   string
	CourseIdentities map[string]string
	DetailCache      map[string]CachedDetail // pre-populated before optimizer runs
}
type Identity struct {
	Id   string `json:"id"`
	Role string `json:"role"`
}

func NewA1CEClient() *A1CEClient {
	client := &A1CEClient{
		BaseURL:          os.Getenv("M2M_BASE_URL"),
		HTTPClient:       &http.Client{Timeout: 10 * time.Second},
		CourseIdentities: make(map[string]string),
	}

	// Generate token
	accessToken, exp, err := client.GenerateInternalToken()
	if err != nil {
		fmt.Printf("ERROR: Token generation failed: %v\n", err)
		return client // still return client safely
	}

	client.JWTToken = accessToken
	client.TokenExpiresAt = time.Unix(exp, 0)

	client.UniversityCode = os.Getenv("UNIVERSITY")
	// Read the JSON file automatically!
	data, err := os.ReadFile("course_identities.json")
	if err == nil {
		json.Unmarshal(data, &client.CourseIdentities)
		fmt.Printf("Successfully loaded %d course identities from JSON.\n", len(client.CourseIdentities))
	} else {
		fmt.Println("Warning: Could not load course_identities.json:", err)
	}

	return client
}
func (c *A1CEClient) GenerateInternalToken() (string, int64, error) {
	expTime := time.Now().Add(24 * time.Hour).Unix()
	privateKeyStr := os.Getenv("M2M_JWT_KEY")
	if privateKeyStr == "" {
		return "", 0, fmt.Errorf("M2M_JWT_KEY is missing from the .env file")
	}

	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\\n", "\n")
	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\"", "")

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyStr))
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse RSA private key: %v", err)
	}

	// Let's grab your one working ID
	adminID := os.Getenv("M2M_CURRICULUM_DESIGNER_ID")

	claims := jwt.MapClaims{
		"email":       os.Getenv("M2M_EMAIL"),
		"given_name":  "CMKL",
		"family_name": "Recommendations",
		"picture":     "",
		"locale":      "",
		"roles":       []string{"User", "Admin", "Curriculum Designer"},
		"user_id":     os.Getenv("M2M_USER_ID"),
		"identities": []map[string]interface{}{
			{
				// Badge 1: Admin (For Student Data)
				"id":   os.Getenv("M2M_ADMIN_ID"),
				"role": "Admin",
			},
			{
				// Badge 2: Curriculum Designer (For Course Catalog)
				"id":   os.Getenv("M2M_CURRICULUM_DESIGNER_ID"),
				"role": "Curriculum Designer",
			},
			// ---> NEW: THE RECOMMENDER BADGE <---
			{
				"id":   adminID, // Use Admin ID as a fallback for the Recommender ID
				"role": "Recommender",
			},
		},
		"exp": expTime,
		"jti": fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	// ---> ADD THESE LINES TO PRINT THE CLAIMS <---
	claimsJSON, _ := json.MarshalIndent(claims, "", "  ")
	fmt.Println("\n--- DEBUG: EXACT JWT CLAIMS BEING SENT ---")
	fmt.Println(string(claimsJSON))
	fmt.Println("------------------------------------------")

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(key)
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, expTime, nil

}

// This function checks whether the current JWT token is still valid.
// If the token is missing or will expire within the next 5 minutes,
// it generates a new token using GenerateInternalToken and updates the client state.
// This prevents API calls from failing due to expired authentication.
// Returns an error if token regeneration fails.
func (c *A1CEClient) ensureValidToken() error {

	// If token is still valid for more than 5 minutes, reuse it
	if c.JWTToken != "" && time.Now().Before(c.TokenExpiresAt.Add(-5*time.Minute)) {
		return nil
	}
	token, exp, err := c.GenerateInternalToken()
	if err != nil {
		return fmt.Errorf("failed to refresh JWT token: %w", err)
	}

	c.JWTToken = token
	c.TokenExpiresAt = time.Unix(exp, 0)

	return nil
}
func (c *A1CEClient) GetStudentProfile(studentID string) (*StudentProfile, error) {
	profile := &StudentProfile{
		StudentID:           studentID,
		Competencies:        make(map[string]float64),
		CourseSemesters:     make(map[string]string),
		CompletedCourses:    []string{},
		DistributionCredits: make(map[string]A1CECredit),
	}

	identity, err := c.getStudentIdentity(studentID)
	if err != nil {
		return nil, err
	}

	profile.UniversityCode = identity.UniversityCode
	c.UniversityCode = identity.UniversityCode
	profile.CurriculumVersion = identity.CurriculumVersion

	cards, err := c.getStudentCompetencies(studentID)
	if err == nil {
		for _, card := range cards {
			profile.Competencies[card.CourseCode] = card.Grade
			if card.Semester != "" {
				profile.CourseSemesters[card.CourseCode] = card.Semester
			}
			if card.Status == "Recorded" || card.Status == "Completed" || card.Grade >= 1.0 {
				// CompletedCourses includes both statuses — used to exclude courses from being recommended again.
				profile.CompletedCourses = append(profile.CompletedCourses, card.CourseCode)
				if card.TemplateID != "" {
					profile.CompletedCourses = append(profile.CompletedCourses, card.TemplateID)
				}
			}
			if card.Status == "Recorded" {
				// RecordedCourses is strictly Recorded — used for prerequisite satisfaction checks.
				profile.RecordedCourses = append(profile.RecordedCourses, card.CourseCode)
				if card.TemplateID != "" {
					profile.RecordedCourses = append(profile.RecordedCourses, card.TemplateID)
				}
			}
		}
	}

	gradStatus, err := c.getGraduationStatus(studentID)
	if err == nil {
		profile.RequiredCompetencies = gradStatus.RequiredCompetencies
		profile.DistributionCredits = gradStatus.DistributionCredits
		profile.TotalCredits = gradStatus.TotalCredits
	}

	return profile, nil
}

func (c *A1CEClient) GetSemesterCompetencies(studentID, semester string) ([]A1CECompetencyCard, error) {
	safeSemester := url.QueryEscape(semester)
	if strings.Contains(semester, " ") && !strings.Contains(safeSemester, "%20") {
		safeSemester = strings.ReplaceAll(semester, " ", "%20")
	}

	url := fmt.Sprintf("%s/student/cards/semester?student_id=%s&semester_name=%s", c.BaseURL, studentID, safeSemester)

	var input struct {
		Info struct {
			Cards []A1CECompetencyCard `json:"cards"`
		} `json:"card_info"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	return input.Info.Cards, nil
}

func (c *A1CEClient) GetCourseCatalog(semester string, curriculumVersion int) (*CourseCatalogResponse, error) {
	subdomains, err := c.getSubdomains(curriculumVersion)
	if err != nil {
		return nil, err
	}

	catalog := &CourseCatalogResponse{
		Status: "success", Semester: semester, CurriculumVersion: curriculumVersion, Courses: []Course{},
	}

	seenIDs := make(map[string]bool)

	for subdomainID, isCore := range subdomains {
		courses, err := c.getCoursesForSubdomain(subdomainID, semester, curriculumVersion, isCore)
		if err != nil {
			fmt.Printf("Warning: Failed to fetch subdomain %s: %v\n", subdomainID, err)
			continue
		}

		for _, course := range courses {
			if !seenIDs[course.CourseID] {
				seenIDs[course.CourseID] = true
				catalog.Courses = append(catalog.Courses, course)

				//Get the competency detail
				// competencyDetail, err := c.getCompetencyDetail(course.CourseCode, semester, curriculumVersion)
				// if err != nil {
				// 	fmt.Printf("Warning: Failed to fetch coompetency Detail %s: %v\n", course.CourseCode, err)
				// 	continue
				// }
				// fmt.Printf("Competency Detail :%v", competencyDetail)

			}
		}
	}

	catalog.TotalCourses = len(catalog.Courses)
	return catalog, nil
}

// --- API CALLS ---

func (c *A1CEClient) getStudentIdentity(studentID string) (*A1CEStudentIdentity, error) {
	// Fixed URL construction
	url := fmt.Sprintf("%s/student/identity?student_id=%s", c.BaseURL, studentID)
	var input struct {
		Student A1CEStudentIdentity `json:"student"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	return &input.Student, nil
}

func (c *A1CEClient) getStudentCompetencies(studentID string) ([]A1CECompetencyCard, error) {
	// Clean URL construction
	url := fmt.Sprintf("%s/student/cards?student_id=%s", c.BaseURL, studentID)
	var input struct {
		Info struct {
			Cards []A1CECompetencyCard `json:"cards"`
		} `json:"card_info"`
	}
	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}
	return input.Info.Cards, nil
}

func (c *A1CEClient) getGraduationStatus(studentID string) (*A1CEGraduationStatus, error) {
	// Clean URL construction
	url := fmt.Sprintf("%s/student/graduation/status?student_id=%s", c.BaseURL, studentID)
	var input struct {
		Status struct {
			RequiredCompetencies interface{} `json:"required_course_not_taken"`
			A1CECreditStatus
		} `json:"graduationstatus"`
	}

	if err := c.makeRequest("GET", url, &input); err != nil {
		return nil, err
	}

	var status A1CEGraduationStatus
	status.A1CECreditStatus = input.Status.A1CECreditStatus
	status.RequiredCompetencies = []string{}

	switch v := input.Status.RequiredCompetencies.(type) {
	case []interface{}:
		for _, item := range v {
			switch val := item.(type) {
			case string:
				status.RequiredCompetencies = append(status.RequiredCompetencies, val)
			case map[string]interface{}:
				if code, ok := val["competency_code"].(string); ok {
					status.RequiredCompetencies = append(status.RequiredCompetencies, code)
				} else if code, ok := val["code"].(string); ok {
					status.RequiredCompetencies = append(status.RequiredCompetencies, code)
				}
			}
		}
	}

	return &status, nil
}

func (c *A1CEClient) getSubdomains(curriculumVersion int) (map[string]bool, error) {
	// We added &university_code=CMKL to the end!
	url := fmt.Sprintf("%s/subdomain?curriculum_version=%d&university_code=CMKL", c.BaseURL, curriculumVersion)

	var response struct {
		Pillars []struct {
			IsCore     bool `json:"is_core"`
			Subdomains []struct {
				ID string `json:"id"`
			} `json:"subdomains"`
		} `json:"pillars"`
	}
	if err := c.makeRequest("GET", url, &response); err != nil {
		return nil, err
	}

	subdomains := make(map[string]bool)
	for _, p := range response.Pillars {
		for _, s := range p.Subdomains {
			if s.ID != "" {
				subdomains[s.ID] = p.IsCore
			}
		}
	}
	return subdomains, nil
}

func (c *A1CEClient) getCoursesForSubdomain(subdomainID, semester string, curriculumVersion int, isPillarCore bool) ([]Course, error) {
	safeSemester := url.QueryEscape(semester)
	if strings.Contains(semester, " ") && !strings.Contains(safeSemester, "%20") {
		safeSemester = strings.ReplaceAll(semester, " ", "%20")
	}

	// We added &university_code=CMKL to the end here too!
	url := fmt.Sprintf("%s/competency?subdomain_id=%s&semester_name=%s&curriculum_version=%d&university_code=CMKL",
		c.BaseURL, subdomainID, safeSemester, curriculumVersion)

	type APICourse struct {
		ID          string  `json:"id"`
		TemplateID  string  `json:"template_id"` // <-- ADD THIS LINE
		Code        string  `json:"competency_code"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Credits     float64 `json:"credits"`
		Semester    string  `json:"semester_offered"`
		IsCore      bool    `json:"is_core"`
		IsRequired  bool    `json:"is_required"`
	}

	var response struct {
		Competencies []APICourse `json:"competencies"`
	}
	if err := c.makeRequest("GET", url, &response); err != nil {
		return nil, err
	}

	var courses []Course
	for _, ac := range response.Competencies {
		finalID := ac.ID
		if finalID == "" {
			finalID = ac.Code
		}

		isCore := ac.IsCore || ac.IsRequired

		// --- THE DICTIONARY LOOKUP ---
		// Start with whatever the API gave us (usually blank)
		finalTemplateID := ac.TemplateID
		// Check our JSON dictionary. If a match exists, overwrite it!
		if mappedID, exists := c.CourseIdentities[ac.Code]; exists {
			finalTemplateID = mappedID
		}

		courses = append(courses, Course{
			CourseID:             finalID,
			TemplateID:           finalTemplateID, // <-- SAVES THE MATCHED ID
			CourseCode:           ac.Code,
			CourseName:           ac.Title,
			Description:          ac.Description,
			CreditHours:          ac.Credits,
			SubdomainID:          subdomainID,
			SemesterOffered:      ac.Semester,
			IsCore:               isCore,
			IsRequired:           ac.IsRequired,
			RequiredCompetencies: make(map[string]float64),
			TeachesCompetencies:  []string{},
			Prerequisites:        []CompetencyPrerequisiteInfo{},
		})
	}
	return courses, nil
}

// This function is to retrive the competency detail information
func (c *A1CEClient) getCompetencyDetail(competencyCode string, semesterName string, curriculumVersion int) (*Course, string, string, bool, bool, []CourseSchedule, error) {
	// Serve from cache if pre-populated — avoids HTTP calls during optimizer packaging.
	if c.DetailCache != nil {
		if cached, ok := c.DetailCache[competencyCode]; ok {
			return nil, cached.StartDate, cached.EndDate, cached.IsAssessmentOnly, cached.IsRequired, cached.Schedule, nil
		}
	}

	u, err := url.Parse(c.BaseURL + "/competency/detail")
	if err != nil {
		return nil, "", "", false, false, nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	q.Set("competency_code", competencyCode)
	q.Set("semester_name", semesterName)
	q.Set("curriculum_version", strconv.Itoa(curriculumVersion))
	q.Set("university_code", c.UniversityCode)
	u.RawQuery = q.Encode()

	finalURL := strings.ReplaceAll(u.String(), "+", "%20")

	// The struct that catches the dates and schedule slots.
	// Schedule fields (weekday, first_week, last_week, start_time, end_time)
	// are flat inside semester_detail per the actual API response.
	type competencyDetailResponse struct {
		ID          string  `json:"id"`
		TemplateID  string  `json:"template_id"`
		Code        string  `json:"competency_code"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Credits     float64 `json:"credits"`
		Required    bool    `json:"required"`
		SemesterDetail struct {
			StartDate      string `json:"start_date"`
			EndDate        string `json:"end_date"`
			AssessmentOnly bool   `json:"assessment_only"`
			Weekday        string `json:"weekday"`
			FirstWeek      int    `json:"first_week"`
			LastWeek       int    `json:"last_week"`
			StartTime      string `json:"start_time"`
			EndTime        string `json:"end_time"`
		} `json:"semester_detail"`
	}

	var response struct {
		Competency competencyDetailResponse `json:"competency"`
	}

	if err := c.makeRequest("GET", finalURL, &response); err != nil {
		return nil, "", "", false, false, nil, fmt.Errorf("failed to get competency detail: %w", err)
	}

	ac := response.Competency

	// Build a single-slot schedule from the flat semester_detail fields.
	var schedule []CourseSchedule
	if ac.SemesterDetail.Weekday != "" {
		schedule = []CourseSchedule{{
			Day:       ac.SemesterDetail.Weekday,
			StartTime: ac.SemesterDetail.StartTime,
			EndTime:   ac.SemesterDetail.EndTime,
			StartWeek: ac.SemesterDetail.FirstWeek,
			EndWeek:   ac.SemesterDetail.LastWeek,
		}}
		fmt.Printf("[SCHEDULE] %s — %s weeks %d-%d %s-%s\n",
			ac.Code, ac.SemesterDetail.Weekday,
			ac.SemesterDetail.FirstWeek, ac.SemesterDetail.LastWeek,
			ac.SemesterDetail.StartTime, ac.SemesterDetail.EndTime)
	} else {
		fmt.Printf("[SCHEDULE] %s — no schedule data\n", ac.Code)
	}

	finalID := ac.ID
	if finalID == "" {
		finalID = ac.Code
	}

	finalTemplateID := ac.TemplateID
	if mappedID, exists := c.CourseIdentities[ac.Code]; exists {
		finalTemplateID = mappedID
	}

	course := &Course{
		CourseID:    finalID,
		TemplateID:  finalTemplateID,
		CourseCode:  ac.Code,
		CourseName:  ac.Title,
		Description: ac.Description,
		CreditHours: ac.Credits,
	}

	return course, ac.SemesterDetail.StartDate, ac.SemesterDetail.EndDate, ac.SemesterDetail.AssessmentOnly, ac.Required, schedule, nil
}

// This function executes an HTTP request to the M2M API with token validation, and response handling.
// - Ensures the JWT access token is valid before sending the request
// - Builds and executes the HTTP request with required headers
// - Logs request URL and response status for debugging
// - Detects and rejects HTML responses (e.g., security gateway blocks)
// - Handles non-2xx HTTP responses as API errors
// - Parses successful JSON responses into the provided result object
// Returns an error if request creation, network call, authentication, or response parsing fails.
func (c *A1CEClient) makeRequest(method, urlStr string, result interface{}) error {

	fmt.Println("\n--------------------------------------------------")
	fmt.Printf("DEBUG: REQUEST URL: %s\n", urlStr)
	fmt.Println("--------------------------------------------------")

	// Ensure the access token is valid before proceeding
	if err := c.ensureValidToken(); err != nil {
		return err
	}

	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return fmt.Errorf("request creation failed: %w", err)
	}

	// Set required request headers to ensure the client is authenticated and the API accepts the request.
	// This includes:
	// - JWT cookie for authentication
	// - User-Agent to mimic a standard browser request
	// - Accept header to allow all response types
	// - Connection header to keep the connection alive

	req.Header.Set("Cookie", "jwt="+c.JWTToken)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/122.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	// Execute request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	bodyStr := string(body)

	fmt.Printf("INFO: HTTP Status: %d\n", resp.StatusCode)

	// Check HTML response more reliable
	if strings.Contains(bodyStr, "<html") || strings.Contains(bodyStr, "<!DOCTYPE") {
		return fmt.Errorf("blocked by security gateway (HTML response)")
	}

	// Handle non-success status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {

		return fmt.Errorf(
			"api error (status %d): %s",
			resp.StatusCode,
			bodyStr,
		)
	}

	// Parse JSON response
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("json decode failed: %w, body: %s", err, bodyStr)
	}

	fmt.Println("INFO: Request successful")
	return nil
}
func (c *A1CEClient) Login() error {
	loginURL := "https://a1ce.cmkl.ac.th/api/auth/login"

	// Create form data instead of JSON
	data := url.Values{}
	data.Set("email", os.Getenv("CMKL_EMAIL"))
	data.Set("password", os.Getenv("CMKL_PASSWORD"))

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	// Change Content-Type to form-urlencoded
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		fmt.Printf("ERROR: Login failed with status %d. Response: %s\n", resp.StatusCode, string(body))
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode login response: %v", err)
	}

	c.JWTToken = result.Token
	fmt.Println("INFO: Login successful. Session token acquired.")
	return nil
}
func (c *A1CEClient) LoadCourseIdentities(filepath string) {
	c.CourseIdentities = make(map[string]string)

	// Read the JSON file
	data, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Println("Warning: Could not load course identities file:", err)
		return
	}

	// Parse it into the dictionary map
	if err := json.Unmarshal(data, &c.CourseIdentities); err != nil {
		fmt.Println("Warning: Failed to parse course identities JSON:", err)
	}
}
