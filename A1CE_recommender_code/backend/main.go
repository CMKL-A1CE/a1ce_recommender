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
var a1ceClient *A1CEClient

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
	//Create a new a1ce client
	a1ceClient = NewA1CEClient()

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

	log.Printf("Scanning %d semesters sequentially for identity codes...", len(uniqueSemesters))

	// --- THE 503 DDOS FIX: SEQUENTIAL LOOP ---
	// Removed the WaitGroup, Mutex, and concurrent go routines.
	for sem := range uniqueSemesters {
		cards, err := client.GetSemesterCompetencies(studentID, sem)
		if err == nil {
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
		// Add a polite 100ms pause to let the M2M staging server breathe!
		time.Sleep(100 * time.Millisecond)
	}

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
	profile, err := a1ceClient.GetStudentProfile(studentID)
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
	a1ceClient.UniversityCode = "CMKL"
	catalog, err := a1ceClient.GetCourseCatalog(semester, curriculumVersion)
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

	profile, err := a1ceClient.GetStudentProfile(req.StudentID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "A1CE_API_ERROR", "Failed to fetch profile", err.Error())
		return
	}

	if profile.CurriculumVersion == 0 {
		profile.CurriculumVersion = 7
	}

	if strings.EqualFold(strings.TrimSpace(req.Semester), strings.TrimSpace(profile.Semester)) {
		sendError(w, http.StatusBadRequest, "INVALID_SEMESTER", "Cannot generate AI recommendations for a student's entering semester due to lack of historical data. Please use the standard first-year roadmap.", "")
		return
	}

	idMap, _ := loadIdentityMap("course_identities.json")
	completedMap := fetchAllCompletedIdentityCodes(a1ceClient, req.StudentID, profile, idMap)

	// Interests
	var successfulCourses []string
	if req.PreviousSemester == "ALL" {
		for courseCode, grade := range profile.Competencies {
			if grade > 1.0 {
				successfulCourses = append(successfulCourses, courseCode)
			}
		}
	} else if req.PreviousSemester != "" {
		semesterCards, err := a1ceClient.GetSemesterCompetencies(req.StudentID, req.PreviousSemester)
		if err == nil {
			for _, card := range semesterCards {
				if card.Grade > 1.0 {
					successfulCourses = append(successfulCourses, card.CourseCode)
				}
			}
		}
	}

	// Catalog
	catalog, err := a1ceClient.GetCourseCatalog(req.Semester, profile.CurriculumVersion)
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

	// --- PARALLEL PREREQUISITE PRE-FETCH ---
	// Fetch prerequisites for every non-completed catalog course concurrently (max 10 at a time).
	// Without this, a student with many remaining courses causes 40-60 sequential HTTP calls
	// which exceeds the server write timeout and produces a socket hang-up in the client.
	prereqBaseURL := os.Getenv("M2M_BASE_URL")
	prereqLookupVersion := profile.CurriculumVersion
	if prereqLookupVersion <= 0 {
		prereqLookupVersion = 7
	}

	prereqCache := make(map[string][]CompetencyPrerequisiteInfo)
	{
		type fetchResult struct {
			code    string
			prereqs []CompetencyPrerequisiteInfo
		}
		sem := make(chan struct{}, 10) // cap at 10 concurrent API calls
		var wg sync.WaitGroup
		results := make(chan fetchResult, len(catalog.Courses))

		for _, c := range catalog.Courses {
			isCompleted := (c.TemplateID != "" && completedMap[normalizeCode(c.TemplateID)]) ||
				completedMap[normalizeCode(c.CourseCode)] ||
				completedMap[normalizeCode(c.CourseID)]
			if isCompleted {
				continue
			}
			wg.Add(1)
			go func(code string) {
				defer wg.Done()
				sem <- struct{}{}
				prereqs := fetchPrerequisites(prereqBaseURL, code, req.Semester, a1ceClient.JWTToken, prereqLookupVersion)
				<-sem
				results <- fetchResult{code: code, prereqs: prereqs}
			}(c.CourseCode)
		}

		// Close results channel once all goroutines finish
		go func() {
			wg.Wait()
			close(results)
		}()
		for r := range results {
			prereqCache[r.code] = r.prereqs
		}
	}

	// --- PREREQUISITE DEBUG LOG ---
	withPrereqs := 0
	for code, prereqs := range prereqCache {
		if len(prereqs) > 0 {
			withPrereqs++
			fmt.Printf("[PREREQ] %s requires: ", code)
			for _, p := range prereqs {
				fmt.Printf("%s ", p.PrerequisiteCompetencyCode)
			}
			fmt.Println()
		}
	}
	fmt.Printf("[PREREQ] Fetched %d non-completed courses. %d have prerequisites, %d have none.\n",
		len(prereqCache), withPrereqs, len(prereqCache)-withPrereqs)
	// ------------------------------
	// -----------------------------------------

	// --- THEME PREREQUISITE PATHFINDING ---
	// Uses the pre-fetched cache — zero extra API calls here.
	pathfindingBoosts := make(map[string]bool)
	var themeKeywords []string

	if req.PreferredTheme != "" {
		cleanTheme := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.PreferredTheme)), "s")
		for mapKey, words := range ThemeKeywords {
			if strings.Contains(cleanTheme, strings.ToLower(mapKey)) || strings.Contains(strings.ToLower(mapKey), cleanTheme) {
				themeKeywords = words
				break
			}
		}

		for _, catalogCourse := range catalog.Courses {
			if completedMap[normalizeCode(catalogCourse.CourseCode)] || completedMap[normalizeCode(catalogCourse.CourseID)] {
				continue
			}
			courseTitleLower := strings.ToLower(catalogCourse.CourseName)
			isThemeMatch := false
			for _, word := range themeKeywords {
				if strings.Contains(courseTitleLower, strings.ToLower(word)) {
					isThemeMatch = true
					break
				}
			}
			if isThemeMatch {
				for _, p := range prereqCache[catalogCourse.CourseCode] {
					cleanPrereq := normalizeCode(p.PrerequisiteCompetencyCode)
					if !completedMap[cleanPrereq] {
						pathfindingBoosts[cleanPrereq] = true
					}
				}
			}
		}
	}
	// ----------------------------------------

	// --- DR. SALLY'S MASTER PRE-FILTER ---
	var validCandidatePool []Course
	filteredCompleted, filteredPrereq, filteredSequence, filteredOther := 0, 0, 0, 0

	for _, course := range catalog.Courses {
		// 1. Is it already completed? Skip it.
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
			filteredCompleted++
			continue
		}

		course.Prerequisites = prereqCache[course.CourseCode]

		if !CheckPrerequisites(course, profile) {
			fmt.Printf("[PREREQ-BLOCKED] %s (%s) — missing: ", course.CourseCode, course.CourseName)
			for _, p := range course.Prerequisites {
				fmt.Printf("%s ", p.PrerequisiteCompetencyCode)
			}
			fmt.Println()
			filteredPrereq++
			continue
		}

		if strings.HasPrefix(course.CourseCode, "SOF-") {
			continue
		}

		if course.SemesterOffered != "" && !strings.EqualFold(course.SemesterOffered, req.Semester) {
			continue
		}

		prefix, num := parseCourseCode(course.CourseCode)
		isBlockedBySequence := false

		if prefix != "" && num > 0 {
			courseFamily := num / 10
			courseStep := num % 10
			for _, lowerCourse := range catalog.Courses {
				lowerPrefix, lowerNum := parseCourseCode(lowerCourse.CourseCode)
				if lowerPrefix == prefix && lowerNum > 0 {
					lowerFamily := lowerNum / 10
					lowerStep := lowerNum % 10
					if lowerFamily == courseFamily && lowerStep < courseStep {
						lowerCompleted := false
						if lowerCourse.TemplateID != "" && completedMap[normalizeCode(lowerCourse.TemplateID)] {
							lowerCompleted = true
						}
						if lowerCourse.CourseName != "" && completedMap["NAME:"+smartCleanName(lowerCourse.CourseName)] {
							lowerCompleted = true
						}
						if completedMap[normalizeCode(lowerCourse.CourseCode)] || completedMap[normalizeCode(lowerCourse.CourseID)] {
							lowerCompleted = true
						}
						if !lowerCompleted {
							isBlockedBySequence = true
							break
						}
					}
				}
			}
		}

		if isBlockedBySequence {
			fmt.Printf("[SEQ-BLOCKED] %s (%s) — earlier course in sequence not completed\n", course.CourseCode, course.CourseName)
			filteredSequence++
			continue
		}

		if prefix == "URD" {
			isEligibleURD := true
			for _, otherCourse := range catalog.Courses {
				otherPrefix, otherNum := parseCourseCode(otherCourse.CourseCode)
				if otherPrefix == "URD" && otherNum < num {
					lowerCompleted := false
					if otherCourse.TemplateID != "" && completedMap[normalizeCode(otherCourse.TemplateID)] {
						lowerCompleted = true
					}
					if completedMap[normalizeCode(otherCourse.CourseCode)] || completedMap[normalizeCode(otherCourse.CourseID)] {
						lowerCompleted = true
					}
					if !lowerCompleted {
						isEligibleURD = false
						break
					}
				}
			}
			if !isEligibleURD {
				filteredOther++
				continue
			}
		}
		validCandidatePool = append(validCandidatePool, course)
	}

	fmt.Printf("[FILTER] Results — passed: %d | completed: %d | prereq-blocked: %d | sequence-blocked: %d | other: %d | catalog total: %d\n",
		len(validCandidatePool), filteredCompleted, filteredPrereq, filteredSequence, filteredOther, len(catalog.Courses))

	// --- NOW PROCESS SCORES FOR THE VALID CANDIDATES ---
	var scoredCourses []RecommendedCourse
	for _, course := range validCandidatePool {
		compScore := CalculateCompetencyMatchScore(course, profile)
		interestScore := CalculateInterestScore(course, profile)
		progScore := CalculateProgramProgressScore(course, profile, requirements)

		fitScore := (compW * compScore) + (intW * interestScore) + (progW * progScore)

		// --- BEHAVIORAL WEIGHT OVERRIDES ---

		// 1. FAST TRACK: Prioritize high-credit courses to accelerate graduation
		if req.WeightType == "fast_track" {
			// A 6-credit course gets +1.8, a 3-credit course gets +0.9 — big enough to reorder rankings
			fitScore += (course.CreditHours * 0.3)
		}

		// 2. PLAY IT SAFE: Boost courses in pillars the student has already succeeded in.
		// +5 so the signal is visible alongside the theme prefix boost (+50): within the
		// chosen theme's courses, familiar ones rank higher; outside the theme, unfamiliar
		// pillars can't catch up to theme courses even with the +5.
		if req.WeightType == "play_it_safe" && len(successfulCourses) > 0 {
			candidatePrefix, _ := parseCourseCode(course.CourseCode)
			for _, success := range successfulCourses {
				successPrefix, _ := parseCourseCode(success)
				if candidatePrefix == successPrefix && candidatePrefix != "" {
					fitScore += 5.0
					break
				}
			}
		}

		// 3. THEME PILLAR BOOST: Boost courses whose code prefix belongs to the chosen theme.
		// Base boost is +50. explore_passions doubles it to +100 so the student's chosen
		// theme dominates even when their course history points elsewhere.
		if req.PreferredTheme != "" {
			cleanTheme := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.PreferredTheme)), "s")
			coursePrefix, _ := parseCourseCode(course.CourseCode)
			themeBoost := 50.0
			if req.WeightType == "explore_passions" {
				themeBoost = 100.0
			}
			for themeKey, prefixes := range ThemePrefixes {
				if strings.Contains(cleanTheme, strings.ToLower(themeKey)) || strings.Contains(strings.ToLower(themeKey), cleanTheme) {
					for _, p := range prefixes {
						if coursePrefix == p {
							fitScore += themeBoost
							break
						}
					}
					break
				}
			}
		}

		// 4. THEME PATHFINDING BOOST: Surface prerequisite courses that unlock blocked theme courses
		if pathfindingBoosts[normalizeCode(course.CourseCode)] {
			fitScore += 100.0 // Highest priority — unlocks a theme course the student can't yet take
		}
		// -----------------------------------

		reason := ""
		weightedComp := compW * compScore
		weightedInt := intW * interestScore
		weightedProg := progW * progScore

		getAdjective := func(score float64) string {
			if score >= 0.25 {
				return "Exceptional"
			} else if score >= 0.15 {
				return "Strong"
			}
			return "Moderate"
		}

		if weightedComp >= weightedInt && weightedComp >= weightedProg {
			reason = fmt.Sprintf("%s Competency Match (Score: %.2f)", getAdjective(compScore), compScore)
		} else if weightedInt >= weightedComp && weightedInt >= weightedProg {
			reason = fmt.Sprintf("%s Interest Alignment (Score: %.2f)", getAdjective(interestScore), interestScore)
		} else {
			reason = fmt.Sprintf("%s Program Progress Value (Score: %.2f)", getAdjective(progScore), progScore)
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

	// --- SORTING LOOP ---
	for i := 0; i < len(scoredCourses); i++ {
		for j := i + 1; j < len(scoredCourses); j++ {
			if scoredCourses[i].FitScore < scoredCourses[j].FitScore {
				scoredCourses[i], scoredCourses[j] = scoredCourses[j], scoredCourses[i]
			}
		}
	}

	// 1. Fetch the graphics right before calling the optimizer
	graphicsMap, _ := fetchPillarGraphics(os.Getenv("M2M_BASE_URL"), a1ceClient.JWTToken)
	if graphicsMap == nil {
		graphicsMap = make(map[string]Graphics)
	}

	// 2. Pass the map into the optimizer
	warningMessage := ""
	var roadmaps []CourseSet

	if profile.TotalCredits.Earned < 36 {
		// 1. The Freshman Block (This still safely stops the algorithm)
		warningMessage = "Student requesting the recommendation has recorded fewer than 36 credits. Not possible to generate recommendations."
	} else {
		combinedStrategy := req.PreferredTheme
		if req.WeightType != "" {
			combinedStrategy = combinedStrategy + " " + req.WeightType
		}

		minScoreCutoff := req.MinFitScore
		if minScoreCutoff <= 0 {
			minScoreCutoff = 0.25
		}

		// Run the optimizer
		roadmaps = OptimizeCourseSets(
			scoredCourses,
			profile,
			requirements,
			req.MaxCreditLoad,
			req.MaxSets,
			combinedStrategy,
			graphicsMap,
			a1ceClient,
			minScoreCutoff,
		)

		if len(roadmaps) == 0 {
			// --- SI THU'S JSON ERROR FIX ---
			sendError(w, http.StatusBadRequest, "NO_ROADMAPS_GENERATED", "No roadmaps generated: could not find enough courses meeting the minimum fit score for the selected theme.", "")
			return
			// -------------------------------
		} else if len(roadmaps) < req.MaxSets {
			// If they asked for 3 but we only got 1 or 2 distinct ones, pass a warning to the UI!
			warningMessage = fmt.Sprintf("Requested %d roadmaps, but could only generate %d distinct option(s) for this specific theme.", req.MaxSets, len(roadmaps))
		}
	}

	// Build the official A1CE Response
	var a1ceRoadmaps []A1CERoadmap

	for i, rm := range roadmaps {
		a1ceRoadmaps = append(a1ceRoadmaps, A1CERoadmap{
			ID:                fmt.Sprintf("roadmap-gen-%d", i),
			Title:             rm.Title,
			Year:              2026,
			Semester:          req.Semester,
			Credits:           rm.TotalCredits,
			AverageScore:      rm.AverageScore,
			MinScore:          rm.MinScore,
			MaxScore:          rm.MaxScore,
			CurriculumVersion: profile.CurriculumVersion,
			MilestoneGroups:   []MilestoneGroup{rm.A1CEMilestoneGroup},
			UniversityCode:    "CMKL",
		})
	}

	// 3. Package the final payload to send back to Si Thu's frontend
	response := A1CEResponse{
		RecommendedRoadmaps: a1ceRoadmaps,
		Status:              "success",
		Warning:             warningMessage,
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

// --- HELPER: Sequential Prerequisite Parser ---
// Safely splits standard codes (e.g. "URD-202", "URD 202", "URD202") into letters and numbers
func parseCourseCode(code string) (string, int) {
	re := regexp.MustCompile(`^([A-Za-z]+)[-\s]*(\d+)`)
	matches := re.FindStringSubmatch(strings.TrimSpace(code))
	if len(matches) >= 3 {
		num, err := strconv.Atoi(matches[2])
		if err == nil {
			return strings.ToUpper(matches[1]), num
		}
	}
	return "", 0
}

// --- BULLETPROOF PREREQUISITE CHECKER (V3: Object-Based) ---
func CheckPrerequisites(course Course, profile *StudentProfile) bool {
	// If the course has no prerequisites attached, they are cleared!
	if len(course.Prerequisites) == 0 {
		return true
	}

	// Create a fast-lookup map of everything the student has completed, stripped of formatting
	completedClean := make(map[string]bool)
	for _, comp := range profile.CompletedCourses {
		cleanComp := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(comp, "-", ""), " ", ""))
		completedClean[cleanComp] = true
	}

	// Loop through the complex Prerequisite Objects
	for _, prereqObj := range course.Prerequisites {
		// Target the specific Code string (e.g., "MAT-211") from the API object
		rawCode := prereqObj.PrerequisiteCompetencyCode
		cleanPrereq := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(rawCode, "-", ""), " ", ""))

		// If they are missing this specific prerequisite, block the course instantly
		if !completedClean[cleanPrereq] {
			return false
		}
	}

	return true
}

// --- HELPER: Fetch Prerequisites ---
func fetchPrerequisites(baseURL string, code string, semester string, token string, currVer int) []CompetencyPrerequisiteInfo {
	if baseURL == "" {
		return nil
	}

	encodedSemester := url.QueryEscape(semester)
	apiURL := fmt.Sprintf("%s/competency/detail?competency_code=%s&university_code=CMKL&curriculum_version=%d&semester_name=%s", baseURL, code, currVer, encodedSemester)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}

	// --- THE SECURITY HEADER FIX ---
	req.Header.Set("Cookie", "jwt="+strings.TrimSpace(token))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/122.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	// -------------------------------

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("(!) WARNING: Prerequisite fetch failed for %s. Status: %d\n", code, resp.StatusCode)
		return nil
	}
	defer resp.Body.Close()

	var detail CompetencyDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil
	}

	return detail.Competency.Prerequisites
}
