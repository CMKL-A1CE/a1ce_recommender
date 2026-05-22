package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Default starting weights
var CurrentWeights = ScoringWeights{
	Competency: 0.4,
	Interest:   0.3,
	Progress:   0.3,
}

// enableCORS securely handles Cross-Origin requests by dynamically echoing the origin
// func enableCORS(next http.HandlerFunc) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		// 1. Get the exact origin of the frontend making the request
// 		origin := r.Header.Get("Origin")

// 		// 2. Dynamically echo the origin back to the browser.
// 		// This guarantees a 100% perfect match, satisfying strict credential rules.
// 		if origin != "" {
// 			w.Header().Set("Access-Control-Allow-Origin", origin)
// 		} else {
// 			w.Header().Set("Access-Control-Allow-Origin", "*")
// 		}

// 		// 3. Explicitly allow credentials (Tokens, Cookies)
// 		w.Header().Set("Access-Control-Allow-Credentials", "true")

// 		// 4. Standard Allowed Methods and Headers
// 		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
// 		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Origin")

// 		// 5. Catch the Preflight "OPTIONS" request and send 200 OK immediately
// 		if r.Method == http.MethodOptions {
// 			w.WriteHeader(http.StatusOK)
// 			return
// 		}

// 		// Move on to the actual function
// 		next.ServeHTTP(w, r)
// 	}
// }

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// 1. Check if it matches our exact allowed list
		if origin == "https://a1ce.cmkl.ac.th" || origin == "https://a1ce-test.cmkl.ac.th" || origin == "http://localhost:3000" || origin == "http://localhost:8080" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			// 2. CRITICAL FIX: If the origin is missing (stripped by Kubernetes) or doesn't match,
			// we forcefully set it to the test environment instead of using a "*" wildcard.
			w.Header().Set("Access-Control-Allow-Origin", "https://a1ce-test.cmkl.ac.th")
		}

		// 3. Explicitly allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// 4. Standard Allowed Methods and Headers
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Origin")

		// 5. Catch the Preflight "OPTIONS" request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Move on to the actual function
		next.ServeHTTP(w, r)
	}
}

func main() {
	godotenv.Load() // Loads the A1CE_JWT_KEY variable
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		if err := EvaluateAllStudentsFromSQLite("a1ce_recommendation.db"); err != nil {
			log.Fatalf("evaluation failed: %v", err)
		}
		return
	}

	rules, err := loadCurriculumRules("curriculum_rules.json")
	if err != nil {
		log.Println("(!) CRITICAL ERROR: Could not load curriculum_rules.json")
	} else {
		count := 0
		for _, req := range rules {
			if req {
				count++
			}
		}
		log.Printf("(✓) SUCCESS: Loaded %d REQUIRED rules from curriculum_rules.json\n", count)
	}

	// Load Identity Map on startup
	idMap, err := loadIdentityMap("course_identities.json")
	if err != nil {
		log.Println("(!) WARNING: Could not load course_identities.json")
	} else {
		log.Printf("(✓) SUCCESS: Loaded %d IDENTITY mappings.", len(idMap))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/recommendations", enableCORS(handleRecommendations))
	mux.HandleFunc("/api/v1/student-data", enableCORS(handleStudentData))
	mux.HandleFunc("/api/v1/course-catalog", enableCORS(handleCourseCatalog))
	mux.HandleFunc("/api/v1/health", enableCORS(handleHealth))
	mux.HandleFunc("/api/v1/weights", enableCORS(handleWeightsUpdate))

	handler := corsMiddleware(loggingMiddleware(authMiddleware(mux)))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("===========================================")
	log.Println("A1CE Course Recommender API Server")
	log.Println("===========================================")
	log.Println("Server listening on: http://localhost:8080")

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// --- HELPER: Fetch Full History ---
func fetchAllCompletedIdentityCodes(client *A1CEClient, studentID string, profile *StudentProfile, idMap map[string]string) map[string]bool {
	completed := make(map[string]bool)

	// 1. Codes from main profile
	for _, c := range profile.CompletedCourses {
		normC := normalizeCode(c)
		completed[normC] = true
		if mappedID, ok := idMap[normC]; ok {
			completed[normalizeCode(mappedID)] = true
		}
	}

	uniqueSemesters := make(map[string]bool)
	for _, sem := range profile.CourseSemesters {
		if sem != "" {
			uniqueSemesters[sem] = true
		}
	}

	log.Printf("Scanning %d semesters for identity codes...", len(uniqueSemesters))

	var wg sync.WaitGroup
	var mu sync.Mutex

	for sem := range uniqueSemesters {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			cards, err := client.GetSemesterCompetencies(studentID, s)
			if err == nil {
				mu.Lock()
				defer mu.Unlock()
				for _, card := range cards {
					completed[normalizeCode(card.CourseCode)] = true
					completed[normalizeCode(card.CompetencyID)] = true
					if card.TemplateID != "" {
						completed[normalizeCode(card.TemplateID)] = true
					}
					if card.CourseName != "" {
						completed["NAME:"+smartCleanName(card.CourseName)] = true
					}
					// Map check
					if mappedID, ok := idMap[normalizeCode(card.CourseCode)]; ok {
						completed[normalizeCode(mappedID)] = true
					}
				}
			}
		}(sem)
	}
	wg.Wait()

	log.Printf("History scan complete. Total unique markers: %d", len(completed))
	return completed
}

func loadIdentityMap(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var mapping map[string]string
	if err := json.NewDecoder(file).Decode(&mapping); err != nil {
		return nil, err
	}
	normalized := make(map[string]string)
	for k, v := range mapping {
		normalized[normalizeCode(k)] = v
	}
	return normalized, nil
}

func loadCurriculumRules(filename string) (map[string]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rules map[string]bool
	if err := json.NewDecoder(file).Decode(&rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func normalizeCode(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimSpace(s)
	return s
}

func smartCleanName(s string) string {
	s = strings.ToLower(s)
	noise := []string{"basic ", "fundamentals of ", "introduction to ", "advanced ", "principles of "}
	for _, n := range noise {
		s = strings.ReplaceAll(s, n, "")
	}
	reg, _ := regexp.Compile("[^a-z0-9]+")
	return reg.ReplaceAllString(s, "")
}

// --- HANDLERS ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func handleStudentData(w http.ResponseWriter, r *http.Request) {
	studentID := r.URL.Query().Get("student_id")
	if studentID == "" {
		sendError(w, http.StatusBadRequest, "MISSING_PARAM", "student_id is required", "")
		return
	}
	client := NewA1CEClient()
	//client.JWTToken = getAuthorzationCred(r, "token")
	profile, err := client.GetStudentProfile(studentID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "API_ERROR", "Failed to fetch student data", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func handleCourseCatalog(w http.ResponseWriter, r *http.Request) {
	semester := r.URL.Query().Get("semester")
	curriculumVersionStr := r.URL.Query().Get("curriculum_version")
	curriculumVersion, err := strconv.Atoi(curriculumVersionStr)
	if semester == "" || err != nil {
		sendError(w, http.StatusBadRequest, "MISSING_REQUIRED_FIELD", "semester/version required", "")
		return
	}
	client := NewA1CEClient()
	client.JWTToken = getAuthorzationCred(r, "token")
	client.UniversityCode = "CMKL"
	catalog, err := client.GetCourseCatalog(semester, curriculumVersion)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch catalog", err.Error())
		return
	}

	idMap, _ := loadIdentityMap("course_identities.json")
	rules, _ := loadCurriculumRules("curriculum_rules.json")
	normRules := make(map[string]bool)
	if rules != nil {
		for code, isReq := range rules {
			if isReq {
				normRules[normalizeCode(code)] = true
			}
		}
	}

	for i := range catalog.Courses {
		c := &catalog.Courses[i]
		normCode := normalizeCode(c.CourseCode)

		// Inject Identity ID
		if val, ok := idMap[normCode]; ok {
			c.TemplateID = val
		}
		// Inject Required Status
		if normRules[normCode] || normRules[normalizeCode(c.CourseID)] {
			c.IsRequired = true
			c.IsCore = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(catalog)
}

func handleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST requests allowed", "")
		return
	}

	var req RecommendationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse request body", err.Error())
		return
	}

	// --- 1. SET UP DYNAMIC WEIGHTS ---
	compW, intW, progW := CurrentWeights.Competency, CurrentWeights.Interest, CurrentWeights.Progress
	if req.WeightType == "fast_track" {
		compW, intW, progW = 0.1, 0.1, 0.8
	} else if req.WeightType == "explore_passions" {
		compW, intW, progW = 0.1, 0.8, 0.1
	} else if req.WeightType == "play_it_safe" {
		compW, intW, progW = 0.8, 0.1, 0.1
	} else if req.WeightType == "balanced" {
		compW, intW, progW = 0.33, 0.33, 0.34
	}

	client := NewA1CEClient()
	client.JWTToken = getAuthorzationCred(r, "token")

	profile, err := client.GetStudentProfile(req.StudentID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch profile", err.Error())
		return
	}

	idMap, _ := loadIdentityMap("course_identities.json")
	completedMap := fetchAllCompletedIdentityCodes(client, req.StudentID, profile, idMap)

	// Interests
	var successfulCourses []string
	if req.PreviousSemester == "ALL" {
		for courseCode, grade := range profile.Competencies {
			if grade > 1.0 {
				successfulCourses = append(successfulCourses, courseCode)
			}
		}
	} else if req.PreviousSemester != "" {
		semesterCards, err := client.GetSemesterCompetencies(req.StudentID, req.PreviousSemester)
		if err == nil {
			for _, card := range semesterCards {
				if card.Grade > 1.0 {
					successfulCourses = append(successfulCourses, card.CourseCode)
				}
			}
		}
	}

	// Catalog
	catalog, err := client.GetCourseCatalog(req.Semester, profile.CurriculumVersion)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch catalog", err.Error())
		return
	}

	// Inject IDs into Catalog
	for i := range catalog.Courses {
		c := &catalog.Courses[i]
		if val, ok := idMap[normalizeCode(c.CourseCode)]; ok {
			c.TemplateID = val
		}
	}

	profile.InterestWeights = make(map[string]float64)
	if len(successfulCourses) > 0 {
		for _, successCode := range successfulCourses {
			parts := strings.Split(successCode, "-")
			if len(parts) > 0 {
				prefix := parts[0]
				for _, course := range catalog.Courses {
					if strings.HasPrefix(course.CourseCode, prefix) {
						profile.InterestWeights[course.SubdomainID] += 5.0
					}
				}
			}
		}
	}
	totalWeight := 0.0
	for _, w := range profile.InterestWeights {
		totalWeight += w
	}
	if totalWeight > 0 {
		for k := range profile.InterestWeights {
			profile.InterestWeights[k] /= totalWeight
		}
	}

	requirements := &CurriculumRequirements{
		CurriculumVersion:    profile.CurriculumVersion,
		RequiredCompetencies: profile.RequiredCompetencies,
		TotalCreditsRequired: float64(profile.TotalCredits.Required),
	}

	isCurriculumReq := make(map[string]bool)
	rules, _ := loadCurriculumRules("curriculum_rules.json")
	if rules != nil {
		for code, rReq := range rules {
			if rReq {
				isCurriculumReq[normalizeCode(code)] = true
			}
		}
	}

	var scoredCourses []RecommendedCourse
	for _, course := range catalog.Courses {
		// --- FILTERING ---
		isCompleted := false
		if course.TemplateID != "" && completedMap[normalizeCode(course.TemplateID)] {
			isCompleted = true
		}
		if course.CourseName != "" {
			cName := "NAME:" + smartCleanName(course.CourseName)
			if completedMap[cName] {
				isCompleted = true
			}
		}
		if completedMap[normalizeCode(course.CourseCode)] {
			isCompleted = true
		}
		if completedMap[normalizeCode(course.CourseID)] {
			isCompleted = true
		}

		if isCompleted {
			continue
		}

		if !CheckPrerequisites(course, profile) {
			continue
		}
		if strings.HasPrefix(course.CourseCode, "SOF-") {
			continue
		}
		if course.SemesterOffered != "" && !strings.EqualFold(course.SemesterOffered, req.Semester) {
			continue
		}

		compScore := CalculateCompetencyMatchScore(course, profile)
		interestScore := CalculateInterestScore(course, profile)
		progScore := CalculateProgramProgressScore(course, profile, requirements)

		// --- 2. APPLY DYNAMIC WEIGHTS TO THE MATH ---
		fitScore := (compW * compScore) + (intW * interestScore) + (progW * progScore)

		// Determine the dominant factor for the Reason string
		reason := ""
		weightedComp := compW * compScore
		weightedInt := intW * interestScore
		weightedProg := progW * progScore

		if weightedComp >= weightedInt && weightedComp >= weightedProg {
			reason = fmt.Sprintf("Strong Competency Match (Score: %.2f)", compScore)
		} else if weightedInt >= weightedComp && weightedInt >= weightedProg {
			reason = fmt.Sprintf("Strong Interest Alignment (Score: %.2f)", interestScore)
		} else {
			reason = fmt.Sprintf("High Program Progress Value (Score: %.2f)", progScore)
		}

		displayCourse := CourseOutput{
			CourseID:             course.CourseID,
			TemplateID:           course.TemplateID,
			CourseCode:           course.CourseCode,
			CourseName:           course.CourseName,
			Description:          course.Description,
			CreditHours:          course.CreditHours,
			SubdomainID:          course.SubdomainID,
			TeachesCompetencies:  course.TeachesCompetencies,
			SemesterOffered:      course.SemesterOffered,
			RequiredCompetencies: make(map[string]string),
		}

		if isCurriculumReq[normalizeCode(course.CourseCode)] ||
			(course.TemplateID != "" && isCurriculumReq[normalizeCode(course.TemplateID)]) {
			displayCourse.RequiredCompetencies["Required"] = "-"
		} else {
			displayCourse.RequiredCompetencies["Not Required"] = "-"
		}
		for _, missing := range profile.RequiredCompetencies {
			if normalizeCode(missing) == normalizeCode(course.CourseCode) {
				displayCourse.RequiredCompetencies["Required"] = "-"
			}
		}

		scoredCourses = append(scoredCourses, RecommendedCourse{
			Course:                 course,
			DisplayCourse:          displayCourse,
			FitScore:               fitScore,
			CompetencyMatchScore:   compScore,
			InterestAlignmentScore: interestScore,
			ProgramProgressScore:   progScore,
			Reason:                 reason,
		})
	}

	for i := 0; i < len(scoredCourses); i++ {
		for j := i + 1; j < len(scoredCourses); j++ {
			if scoredCourses[i].FitScore < scoredCourses[j].FitScore {
				scoredCourses[i], scoredCourses[j] = scoredCourses[j], scoredCourses[i]
			}
		}
	}

	// 1. Fetch the graphics right before calling the optimizer
	graphicsMap, _ := fetchPillarGraphics(os.Getenv("M2M_STAGING_API_BASE"), client.JWTToken)
	if graphicsMap == nil {
		graphicsMap = make(map[string]Graphics)
	}

	// 2. Pass the map into the optimizer
	warningMessage := ""
	var roadmaps []CourseSet

	if profile.TotalCredits.Earned < 36 {
		// 1. The Freshman Block (This still safely stops the algorithm)
		warningMessage = "Need a total minimum credit of 36 to generate recommendation."
	} else {
		// Run the optimizer
		roadmaps = OptimizeCourseSets(
			scoredCourses,
			profile,
			requirements,
			req.MaxCreditLoad,
			req.MaxSets,
			req.PreferredTheme,
			graphicsMap,
			os.Getenv("M2M_STAGING_API_BASE"),
			client.JWTToken,
		)

		// 2. The 0.5 Soft Cutoff Warning
		// Loop through the generated milestones to see if any missed the 0.5 mark
		cutoffMissed := false
		for _, rm := range roadmaps {
			for _, ms := range rm.A1CEMilestoneGroup.Milestones {
				if ms.FitScore < 0.5 {
					cutoffMissed = true
					break
				}
			}
		}

		if cutoffMissed {
			warningMessage = "Warning: Some recommended competencies did not achieve the 0.5 minimum fit score cutoff."
		}
	}

	// Build the official A1CE Response
	var a1ceRoadmaps []A1CERoadmap

	for i, rm := range roadmaps {
		// Notice how clean this is now! We deleted the redundant color/title loop
		// because OptimizeCourseSets already did it perfectly.

		a1ceRoadmaps = append(a1ceRoadmaps, A1CERoadmap{
			ID:              fmt.Sprintf("roadmap-gen-%d", i),
			Title:           rm.Title, // <--- Grabs Dr. Sally's new ordinal title directly!
			Year:            2026,
			Semester:        req.Semester,
			Credits:         rm.TotalCredits,
			AverageScore:    rm.AverageScore,
			MinScore:        rm.MinScore,
			MaxScore:        rm.MaxScore,
			MilestoneGroups: []MilestoneGroup{rm.A1CEMilestoneGroup},
			UniversityCode:  "CMKL",
		})
	}

	// 3. Package the final payload to send back to Si Thu's frontend
	response := A1CEResponse{
		RecommendedRoadmaps: a1ceRoadmaps,
		Status:              "success",
		Warning:             warningMessage, // <--- Passes the text warning to the UI
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ... (Standard Helpers: containsString, min, sendError, getAuthorzationCred, corsMiddleware, loggingMiddleware, authMiddleware) ...
func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sendError(w http.ResponseWriter, statusCode int, errorCode, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "error",
		"error_code": errorCode,
		"message":    message,
		"details":    details,
	})
}

func getAuthorzationCred(r *http.Request, target_type string) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Completed in %v", time.Since(start))
	})
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// 1. The Request Struct (Must be outside the function)
type WeightsUpdateRequest struct {
	Competency float64 `json:"competency_weight,omitempty"`
	Interest   float64 `json:"interest_weight,omitempty"`
	Progress   float64 `json:"progress_weight,omitempty"`
	WeightType string  `json:"weight_type,omitempty"`
}

// 2. The Function (Notice the opening curly bracket at the end of this line!)
func handleWeightsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "success",
			"current_weights": CurrentWeights,
		})
		return
	}

	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and POST requests allowed", "")
		return
	}

	var req WeightsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse request body", err.Error())
		return
	}

	if req.WeightType != "" {
		if req.WeightType == "fast_track" {
			CurrentWeights = ScoringWeights{0.1, 0.1, 0.8}
		} else if req.WeightType == "explore_passions" {
			CurrentWeights = ScoringWeights{0.1, 0.8, 0.1}
		} else if req.WeightType == "play_it_safe" {
			CurrentWeights = ScoringWeights{0.8, 0.1, 0.1}
		} else if req.WeightType == "balanced" {
			CurrentWeights = ScoringWeights{0.33, 0.33, 0.34}
		}
	} else {
		CurrentWeights = ScoringWeights{
			Competency: req.Competency,
			Interest:   req.Interest,
			Progress:   req.Progress,
		}
	}

	// Build the success response for the weights endpoint
	response := map[string]interface{}{
		"status":          "success",
		"message":         "Algorithm scoring weights updated successfully",
		"current_weights": CurrentWeights,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// fetchPillarGraphics creates a lookup map of Prefix (e.g., "AIC") -> Graphics struct
func fetchPillarGraphics(baseURL string, token string) (map[string]Graphics, error) {
	req, err := http.NewRequest("GET", baseURL+"/api/pillars", nil)
	if err != nil {
		return nil, err
	}

	// Pass the user's authorization token to access the endpoint
	req.Header.Set("Authorization", token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var pillars []PillarInfo
	if err := json.NewDecoder(resp.Body).Decode(&pillars); err != nil {
		return nil, err
	}

	// Build the dictionary: Map the 3-letter prefix to the full Graphics object
	graphicsMap := make(map[string]Graphics)
	for _, p := range pillars {
		prefix := strings.ToUpper(p.PillarPrefix)
		graphicsMap[prefix] = p.PillarGraphics
	}

	return graphicsMap, nil
}

// fetchCompetencyDates calls the /detail API to grab the start and end dates
func fetchCompetencyDates(baseURL string, code string, semester string, token string) (string, string) {
	if baseURL == "" {
		log.Println("(!) ERROR: baseURL is empty. Check your .env file!")
		return "", ""
	}

	encodedSemester := url.QueryEscape(semester)
	apiURL := fmt.Sprintf("%s/api/competency/detail?competency_code=%s&university_code=CMKL&curriculum_version=7&semester_name=%s", baseURL, code, encodedSemester)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("(!) Failed to create request for %s: %v", code, err)
		return "", ""
	}

	// --- ADD THE HEADERS ---
	req.Header.Set("Authorization", "Bearer "+token) // Ensure 'Bearer ' is included if the API requires it
	req.Header.Set("Content-Type", "application/json")

	// --- INITIALIZE THE CLIENT (This fixes the undefined error) ---
	client := &http.Client{
		Timeout: 10 * time.Second, // Good practice to prevent hanging
	}

	// --- EXECUTE ---
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("(!) Request failed for %s: %v", code, err)
		return "", ""
	}
	defer resp.Body.Close()

	// Check if the API is actually saying OK
	if resp.StatusCode != 200 {
		log.Printf("(!) API returned error %d for course %s", resp.StatusCode, code)
		return "", ""
	}

	var detail CompetencyDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		log.Printf("(!) Failed to decode JSON for %s: %v", code, err)
		return "", ""
	}

	// Return the two extracted dates
	return detail.Competency.SemesterDetail.StartDate, detail.Competency.SemesterDetail.EndDate
}
