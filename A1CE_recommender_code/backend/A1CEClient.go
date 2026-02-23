package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type A1CEClient struct {
	BaseURL          string
	HTTPClient       *http.Client
	JWTToken         string
	UniversityCode   string
	CourseIdentities map[string]string
}

func (c *A1CEClient) GenerateInternalToken() (string, error) {
	privateKeyStr := os.Getenv("A1CE_JWT_KEY")
	if privateKeyStr == "" {
		return "", fmt.Errorf("A1CE_JWT_KEY is missing from the .env file")
	}

	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\\n", "\n")
	privateKeyStr = strings.ReplaceAll(privateKeyStr, "\"", "")

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyStr))
	if err != nil {
		return "", fmt.Errorf("failed to parse RSA private key: %v", err)
	}

	claims := jwt.MapClaims{
		"email":       os.Getenv("M2M_EMAIL"),
		"given_name":  "CMKL",
		"family_name": "Recommendations",
		"locale":      "",
		"roles":       []string{"User", "Admin", "Curriculum Designer"},
		"user_id":     "48646235-c3f0-48a4-b28e-126fca224b96",
		"identities": []map[string]interface{}{
			{
				// Badge 1: Admin (For Student Data)
				"id":   os.Getenv("M2M_ADMIN_ID"), // "7f9d2735-f811-431d-b98c-02639dd992d5"
				"role": "Admin",
			},
			{
				// Badge 2: Curriculum Designer (For Course Catalog)
				"id":   "8059fdb1-8122-4c06-b7a8-824621426975",
				"role": "Curriculum Designer",
			},
		},
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"jti": fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	// ---> ADD THESE LINES TO PRINT THE CLAIMS <---
	claimsJSON, _ := json.MarshalIndent(claims, "", "  ")
	fmt.Println("\n--- DEBUG: EXACT JWT CLAIMS BEING SENT ---")
	fmt.Println(string(claimsJSON))
	fmt.Println("------------------------------------------")

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}

func NewA1CEClient() *A1CEClient {
	client := &A1CEClient{
		BaseURL:          "https://a1ce.cmkl.ac.th/api",
		HTTPClient:       &http.Client{Timeout: 10 * time.Second},
		CourseIdentities: make(map[string]string),
	}

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
				profile.CompletedCourses = append(profile.CompletedCourses, card.CourseCode)
				if card.TemplateID != "" {
					profile.CompletedCourses = append(profile.CompletedCourses, card.TemplateID)
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
			Prerequisites:        []string{},
		})
	}
	return courses, nil
}

func (c *A1CEClient) makeRequest(method, url string, result interface{}) error {
	fmt.Println("\n--------------------------------------------------")
	fmt.Printf("DEBUG: ATTEMPTING CONNECTION - %s\n", url)

	// 1. Generate the JWT
	token, err := c.GenerateInternalToken()
	if err != nil {
		fmt.Printf("ERROR: Token Generation Failed: %v\n", err)
		return err
	}

	// 2. Output the JWT for verification
	fmt.Printf("DEBUG: GENERATED JWT: %s\n", token)
	fmt.Println("--------------------------------------------------")

	// 3. Create the HTTP Request
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		fmt.Printf("ERROR: Request Creation Failed: %v\n", err)
		return err
	}

	// 4. Act like a Backend Server, NOT a Web Browser!
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CMKL-Recommender-Service/1.0")

	// 5. Attach Authentication (Both Header and Cookie to be safe)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{
		Name:  "jwt",
		Value: token,
		Path:  "/",
	})

	// 5.5 THE CLOUDFLARE BYPASS: Inject the VIP Headers from .env
	cfHeader := os.Getenv("CF_BYPASS_HEADER")
	cfValue := os.Getenv("CF_BYPASS_VALUE")
	if cfHeader != "" && cfValue != "" {
		req.Header.Set(cfHeader, cfValue)
		fmt.Printf("DEBUG: Injected Cloudflare VIP Bypass Header: %s\n", cfHeader)
	}

	fmt.Println("INFO: Sending request to A1CE server...")

	// 6. Execute the Request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		fmt.Printf("ERROR: Network Request Failed: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("INFO: Response Received - Status: %d\n", resp.StatusCode)

	// 7. Read the Response Body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ERROR: Failed to read response body: %v\n", err)
		return err
	}

	// 8. Detect if the server sent HTML (meaning a redirect to login or Cloudflare block)
	responseStr := string(body)
	if len(responseStr) > 0 && responseStr[0] == '<' {
		fmt.Println("ERROR: Connection blocked by Security Gateway (HTML Received).")

		previewLen := 500
		if len(responseStr) < 500 {
			previewLen = len(responseStr)
		}
		fmt.Printf("DEBUG: HTML Preview: %s\n", responseStr[:previewLen])
		return fmt.Errorf("A1CE API returned HTML - check if RSA Key/ID pair is authorized")
	}

	// 9. Handle non-200 Status Codes
	if resp.StatusCode != 200 {
		fmt.Printf("ERROR: API rejected request (Status %d): %s\n", resp.StatusCode, responseStr)
		return fmt.Errorf("api error %d", resp.StatusCode)
	}

	// 10. Successfully Decode the JSON data
	fmt.Println("INFO: Connection Successful. Data received.")
	err = json.Unmarshal(body, result)
	if err != nil {
		fmt.Printf("ERROR: JSON Decode Failed: %v\n", err)
		return err
	}

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
