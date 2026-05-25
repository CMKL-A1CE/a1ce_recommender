package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"io"
	// any other imports you have here
)

// fetchCompetencyDetailsFromM2M calls the API and returns StartDate, EndDate, IsAssessmentOnly, IsRequired, and Graphics
func fetchCompetencyDetailsFromM2M(baseURL string, code string, semester string, token string) (string, string, bool, bool, Graphics) {
	cleanBaseURL := strings.TrimSpace(baseURL)
	cleanBaseURL = strings.TrimSuffix(cleanBaseURL, "/")

	if cleanBaseURL == "" {
		fmt.Println("(!) ERROR: baseURL is empty!")
		return "", "", false, false, Graphics{}
	}

	encodedSemester := url.QueryEscape(semester)
	apiURL := fmt.Sprintf("%s/api/competency/detail?competency_code=%s&university_code=CMKL&curriculum_version=&semester_name=%s", cleanBaseURL, code, encodedSemester)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Printf("(!) Failed to create request: %v\n", err)
		return "", "", false, false, Graphics{}
	}

	// --- CHRISTINE'S EXACT SECURITY HEADERS ---
	req.Header.Set("Cookie", "jwt="+strings.TrimSpace(token)) // Uses Cookie instead of Bearer!
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/122.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Printf("(!) Request failed for %s: %v\n", code, err)
		return "", "", false, false, Graphics{}
	}
	if resp.StatusCode != 200 {
		fmt.Printf("(!) API BLOCKED US for %s - Status Code: %d\n", code, resp.StatusCode)
		return "", "", false, false, Graphics{}
	}
	defer resp.Body.Close()

	// --- THE DEBUG VISION BLOCK ---
	// Read the raw text instead of silently decoding it
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading body:", err)
		return "", "", false, false, Graphics{}
	}

	// Print the exact raw text from the server to your terminal!
	fmt.Printf("\n--- RAW API RESPONSE FOR %s ---\n", code)
	fmt.Println(string(bodyBytes))
	fmt.Println("----------------------------------")

	// Now decode it safely
	var detail CompetencyDetailResponse
	if err := json.Unmarshal(bodyBytes, &detail); err != nil {
		fmt.Printf("(!) JSON Decode Error for %s: %v\n", code, err)
		return "", "", false, false, Graphics{}
	}

	return detail.Competency.SemesterDetail.StartDate,
		detail.Competency.SemesterDetail.EndDate,
		detail.Competency.SemesterDetail.AssessmentOnly,
		detail.Competency.Required,
		detail.Competency.Graphics
}

// OptimizeCourseSets generates up to 4 thematic roadmaps
// OptimizeCourseSets generates up to 4 thematic roadmaps
func OptimizeCourseSets(
	baseScoredCourses []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
	maxCreditLoad float64,
	maxSets int,
	preferredTheme string,
	graphicsMap map[string]Graphics,
	a1ceClient *A1CEClient, // <--- Correctly passing the client here!
) []CourseSet {

	if graphicsMap == nil {
		graphicsMap = make(map[string]Graphics)
	}
	if len(graphicsMap) == 0 {
		graphicsMap["AIC"] = Graphics{IconBg: "#E51F24", BorderColor: "#B91216", Icon: ""} 
		graphicsMap["HCD"] = Graphics{IconBg: "#FFC90E", BorderColor: "#D6A300", Icon: ""} 
		graphicsMap["SYS"] = Graphics{IconBg: "#ED4C7B", BorderColor: "#C6305C", Icon: ""} 
		graphicsMap["SEC"] = Graphics{IconBg: "#17C3B2", BorderColor: "#0FA394", Icon: ""} 
		graphicsMap["ENI"] = Graphics{IconBg: "#5AB05B", BorderColor: "#438F44", Icon: ""} 
		graphicsMap["MAT"] = Graphics{IconBg: "#3C78D8", BorderColor: "#2A5CA8", Icon: ""} 
		graphicsMap["SCI"] = Graphics{IconBg: "#0A5C55", BorderColor: "#06423D", Icon: ""} 
		graphicsMap["HAS"] = Graphics{IconBg: "#9628D4", BorderColor: "#751AA8", Icon: ""} 
		graphicsMap["COM"] = Graphics{IconBg: "#F58220", BorderColor: "#CE6813", Icon: ""} 
		graphicsMap["SOF"] = Graphics{IconBg: "#2C1E5C", BorderColor: "#1A103C", Icon: ""} 
		graphicsMap["SEN"] = Graphics{IconBg: "#8C6E51", BorderColor: "#6B523A", Icon: ""} 
		graphicsMap["URD"] = Graphics{IconBg: "#3B14E6", BorderColor: "#260AA3", Icon: ""} 
	}

	// --- DR. SALLY'S < 36 CREDITS CHECK ---
	if studentProfile.TotalCredits.Earned < 36 {
		return []CourseSet{}
	}
	// --------------------------------------

	if maxSets <= 0 {
		maxSets = 1
	}
	if maxSets > 4 {
		maxSets = 4
	}

	defaultThemes := []string{"code", "science", "games", "business"}
	var roadmaps []CourseSet

	for i := 0; i < maxSets; i++ {
		// 1. Figure out the base theme
		currentTheme := strings.ToLower(strings.TrimSpace(preferredTheme))
		displayTheme := strings.ToUpper(currentTheme)

		if currentTheme == "" || currentTheme == "none" {
			genericNames := []string{"BALANCED FOUNDATION", "GENERAL EXPLORATION", "CORE COMPETENCIES", "BROAD FOCUS"}
			if i < len(genericNames) {
				displayTheme = genericNames[i]
			} else {
				displayTheme = "GENERAL EXPLORATION"
			}
			currentTheme = "No Theme"
		} else if currentTheme == "" && i < len(defaultThemes) {
			displayTheme = strings.ToUpper(defaultThemes[i])
		}

		roadmapTitle := fmt.Sprintf("PERSONALIZED ROADMAP %d - %s", i+1, displayTheme)

		iterationCourses := make([]RecommendedCourse, len(baseScoredCourses))
		copy(iterationCourses, baseScoredCourses)

		if currentTheme != "" {
			keywords := ThemeKeywords[strings.ToLower(currentTheme)]
			for idx, courseRec := range iterationCourses {
				course := courseRec.Course
				for _, word := range keywords {
					if strings.Contains(strings.ToLower(course.CourseCode), strings.ToLower(word)) ||
						strings.Contains(strings.ToLower(course.SubdomainID), strings.ToLower(word)) {
						iterationCourses[idx].FitScore += 100.0
						break
					}
				}
			}
		}

		sort.Slice(iterationCourses, func(a, b int) bool {
			return iterationCourses[a].FitScore > iterationCourses[b].FitScore
		})

		if len(iterationCourses) > i && preferredTheme != "" {
			iterationCourses = iterationCourses[i:]
		}

		var selectedCourses []RecommendedCourse
		totalCredits := 0.0
		targetCredits := maxCreditLoad
		subdomainCount := make(map[string]int)
		maxPerSubdomain := 10

		type apiData struct {
			startDate  string
			endDate    string
			isRequired bool
			graphics   Graphics
		}
		liveDataCache := make(map[string]apiData)

		// 1. SELECTION & FILTERING LOOP
		for _, courseRec := range iterationCourses {
			course := courseRec.Course

			if !CheckPrerequisites(course, studentProfile) {
				continue
			}
			if containsRecommendedCourse(selectedCourses, courseRec) {
				continue
			}
			if totalCredits+course.CreditHours > targetCredits {
				continue
			}
			if subdomainCount[course.SubdomainID] >= maxPerSubdomain {
				continue
			}

			// --- ONE SINGLE CLEAN CALL TO THE API ---
			_, startDate, endDate, isAssessmentOnly, isReq, err := a1ceClient.getCompetencyDetail(course.CourseCode, "Spring 2026", studentProfile.CurriculumVersion)
			
			if err != nil {
				fmt.Printf("(!) Error fetching detail via client for %s: %v\n", course.CourseCode, err)
				continue 
			}

			var apiGraphics Graphics 

			if isAssessmentOnly {
				continue
			}

			liveDataCache[course.CourseCode] = apiData{
				startDate:  startDate,
				endDate:    endDate,
				isRequired: isReq,
				graphics:   apiGraphics,
			}
			// ----------------------------------------

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++

			if totalCredits >= targetCredits {
				break
			}
		}

		// 2. PACKAGING LOOP
		var sumScore, minScore, maxScore, avgScore float64
		var milestones []Milestone

		if len(selectedCourses) > 0 {
			minScore = selectedCourses[0].FitScore
			maxScore = selectedCourses[0].FitScore

			for _, c := range selectedCourses {
				sumScore += c.FitScore
				if c.FitScore < minScore {
					minScore = c.FitScore
				}
				if c.FitScore > maxScore {
					maxScore = c.FitScore
				}

				data := liveDataCache[c.Course.CourseCode]

				prefix := ""
				if len(c.Course.CourseCode) >= 3 {
					prefix = strings.ToUpper(c.Course.CourseCode[:3])
				}

				var pillarGraphics Graphics
				if data.graphics.IconBg != "" {
					pillarGraphics = data.graphics
				} else if g, exists := graphicsMap[prefix]; exists {
					pillarGraphics = g
				} else {
					pillarGraphics = Graphics{IconBg: "#f3f4f6", BorderColor: "#9ca3af"}
				}

				finalScore := c.FitScore
				dynamicReason := c.Reason

				if finalScore >= 100.0 {
					finalScore = finalScore - 100.0
					if currentTheme == "No Theme" {
						dynamicReason = "Highly recommended for a balanced foundation."
					} else {
						dynamicReason = fmt.Sprintf("Highly recommended for your %s focus!", strings.ToTitle(currentTheme))
					}
				} else if dynamicReason == "" || strings.Contains(dynamicReason, "0.70") {
					dynamicReason = "Fulfills core curriculum requirements."
				}

				milestones = append(milestones, Milestone{
					ID:                   c.Course.CourseID,
					TemplateID:           c.Course.TemplateID,
					Title:                c.Course.CourseName,
					CompetencyTitle:      c.Course.CourseName,
					CompetencyCode:       c.Course.CourseCode,
					Credits:              int(c.Course.CreditHours),
					SubdomainTitle:       c.Course.SubdomainID,
					FitScore:             finalScore,
					Reason:               dynamicReason,
					Graphics:             pillarGraphics,
					StartDate:            data.startDate,
					TargetCompletionDate: data.endDate,
					Required:             data.isRequired,
				})
			}
			avgScore = sumScore / float64(len(selectedCourses))
		}

		group := MilestoneGroup{
			ID:         "group-auto-gen",
			Title:      "Personalized Recommendations",
			MaxCredits: int(totalCredits),
			Milestones: milestones,
		}

		roadmaps = append(roadmaps, CourseSet{
			Title:              roadmapTitle,
			Theme:              currentTheme,
			Courses:            selectedCourses,
			AverageScore:       avgScore,
			MinScore:           minScore,
			MaxScore:           maxScore,
			TotalCredits:       int(totalCredits),
			A1CEMilestoneGroup: group,
		})
	}

	return roadmaps
}

// EvaluateRecommendationSet calculates quality metrics
func EvaluateRecommendationSet(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) *EvaluationMetrics {
	skillCoverage := CalculateSkillCoverage(recommendedSet, studentProfile, requirements)
	prereqCompliance := CalculatePrerequisiteCompliance(recommendedSet, studentProfile)
	programProgressFit := CalculateProgramProgressFit(recommendedSet, studentProfile, requirements)

	goodnessScore := 0.3*skillCoverage +
		0.3*prereqCompliance +
		0.4*programProgressFit

	return &EvaluationMetrics{
		GoodnessScore:           goodnessScore,
		SkillCoveragePercentage: skillCoverage,
		PrerequisiteCompliance:  prereqCompliance,
		ProgramProgressFit:      programProgressFit,
	}
}

// CalculateSkillCoverage measures percentage of skill gaps addressed
func CalculateSkillCoverage(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) float64 {
	requiredComps := requirements.RequiredCompetencies
	currentComps := getMapKeys(studentProfile.Competencies)
	missingComps := difference(requiredComps, currentComps)

	if len(missingComps) == 0 {
		return 1.0
	}

	var coveredByRecommendations []string
	for _, courseRec := range recommendedSet {
		taughtComps := courseRec.Course.TeachesCompetencies
		coveredByRecommendations = append(coveredByRecommendations, taughtComps...)
	}

	addressedGaps := intersection(missingComps, coveredByRecommendations)
	return float64(len(addressedGaps)) / float64(len(missingComps))
}

// CalculatePrerequisiteCompliance verifies all prerequisites are satisfied
func CalculatePrerequisiteCompliance(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
) float64 {
	if len(recommendedSet) == 0 {
		return 1.0
	}

	compliantCourses := 0
	for _, courseRec := range recommendedSet {
		if CheckPrerequisites(courseRec.Course, studentProfile) {
			compliantCourses++
		}
	}

	return float64(compliantCourses) / float64(len(recommendedSet))
}

// CalculateProgramProgressFit measures degree completion advancement
func CalculateProgramProgressFit(
	recommendedSet []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
) float64 {
	requiredComps := requirements.RequiredCompetencies
	currentComps := getMapKeys(studentProfile.Competencies)
	missingRequired := difference(requiredComps, currentComps)

	competencyProgress := 1.0
	if len(missingRequired) > 0 {
		var covered []string
		for _, courseRec := range recommendedSet {
			taught := courseRec.Course.TeachesCompetencies
			covered = append(covered, intersection(taught, missingRequired)...)
		}
		covered = unique(covered)
		competencyProgress = float64(len(covered)) / float64(len(missingRequired))
	}

	var distributionProgressScores []float64
	distributionCoverage := make(map[string]float64)
	for _, courseRec := range recommendedSet {
		subdomain := courseRec.Course.SubdomainID
		distributionCoverage[subdomain] += courseRec.Course.CreditHours
	}

	for subdomain, requiredCredits := range requirements.DistributionRequirements {
		currentCredits := 0.0
		if credits, exists := studentProfile.DistributionCredits[subdomain]; exists {
			currentCredits = float64(credits.Earned)
		}

		recommendedCredits := 0.0
		if credits, exists := distributionCoverage[subdomain]; exists {
			recommendedCredits = credits
		}

		remainingGap := math.Max(0, requiredCredits-currentCredits)
		areaProgress := 1.0
		if remainingGap > 0 {
			areaProgress = math.Min(1.0, recommendedCredits/remainingGap)
		}
		distributionProgressScores = append(distributionProgressScores, areaProgress)
	}

	distributionProgress := 1.0
	if len(distributionProgressScores) > 0 {
		distributionProgress = mean(distributionProgressScores)
	}

	return 0.6*competencyProgress + 0.4*distributionProgress
}

// --- Helper Functions (Only those specific to optimizer) ---

func containsRecommendedCourse(courses []RecommendedCourse, course RecommendedCourse) bool {
	for _, c := range courses {
		if c.Course.CourseID == course.Course.CourseID {
			return true
		}
	}
	return false
}

func unique(slice []string) []string {
	keys := make(map[string]bool)
	var result []string
	for _, entry := range slice {
		if _, exists := keys[entry]; !exists {
			keys[entry] = true
			result = append(result, entry)
		}
	}
	return result
}
