package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
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
	mux.HandleFunc("/api/v1/recommendations", handleRecommendations)
	mux.HandleFunc("/api/v1/student-data", handleStudentData)
	mux.HandleFunc("/api/v1/course-catalog", handleCourseCatalog)
	mux.HandleFunc("/api/v1/health", handleHealth)

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
	client.JWTToken = getAuthorzationCred(r, "token")
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
		for code, req := range rules {
			if req {
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
		fitScore := 0.2*compScore + 0.6*interestScore + 0.2*progScore

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
			Reason:                 fmt.Sprintf("Interest Score: %.2f", interestScore),
		})
	}

	for i := 0; i < len(scoredCourses); i++ {
		for j := i + 1; j < len(scoredCourses); j++ {
			if scoredCourses[i].FitScore < scoredCourses[j].FitScore {
				scoredCourses[i], scoredCourses[j] = scoredCourses[j], scoredCourses[i]
			}
		}
	}
	recommendedSet := OptimizeCourseSet(scoredCourses, profile, requirements, req.MaxCreditLoad)

	totalCredits := 0.0
	for _, r := range recommendedSet {
		totalCredits += r.Course.CreditHours
	}

	warningMsg := ""
	if req.MaxCreditLoad > 60 {
		warningMsg = "The student is currently doing a credit overload, make sure to already contact CMKL staff"
	}

	response := RecommendationSet{
		StudentID:      req.StudentID,
		Semester:       req.Semester,
		RecommendedSet: recommendedSet,
		TotalCredits:   totalCredits,
		Metrics:        EvaluationMetrics{GoodnessScore: 0.85},
		Metadata: RecommendationMetadata{
			GenerationTimestamp: time.Now(),
			AlgorithmVersion:    "1.31-Identity-JSON-Label",
		},
		Status:  "success",
		Warning: warningMsg,
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
