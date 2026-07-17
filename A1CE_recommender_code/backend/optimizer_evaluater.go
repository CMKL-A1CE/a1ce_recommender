package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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
func OptimizeCourseSets(
	baseScoredCourses []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
	maxCreditLoad float64,
	maxSets int,
	preferredTheme string,
	graphicsMap map[string]Graphics,
	a1ceClient *A1CEClient, // <--- Correctly passing the client here!
	minRequiredScore float64,
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
		graphicsMap["SOF"] = Graphics{IconBg: "#8C6E51", BorderColor: "#6B523A", Icon: ""}
		graphicsMap["SEN"] = Graphics{IconBg: "#5E2B97", BorderColor: "#451B75", Icon: ""}
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

	//defaultThemes := []string{"code", "science", "games", "business"}
	var roadmaps []CourseSet

	// --- NEW: Track unique roadmaps to prevent duplicates ---
	seenSignatures := make(map[string]bool)

	for i := 0; i < maxSets; i++ {
		// 1. Capture the raw string FIRST for the math checks
		rawTheme := strings.ToLower(strings.TrimSpace(preferredTheme))

		// 2. SET WEIGHTS: Check the raw string before we delete any words!
		//minRequiredScore := 0.25 // Default fallback

		//if strings.Contains(rawTheme, "fast_track") {
		//	minRequiredScore = 0.10
		//} else if strings.Contains(rawTheme, "explore_passions") {
		//	minRequiredScore = 0.12
		//} else if strings.Contains(rawTheme, "plat_it_safe") {
		//	minRequiredScore = 0.50
		//} else if strings.Contains(rawTheme, "custom") {
		//	minRequiredScore = 0.10 // Custom weights bypass
		//}

		// 3. UI NAMING (Dr. Sally's Rules)
		// Clean the string so we don't print internal tags to the frontend
		cleanTitle := strings.ToUpper(rawTheme)
		cleanTitle = strings.ReplaceAll(cleanTitle, "NONE", "")
		cleanTitle = strings.ReplaceAll(cleanTitle, "CUSTOM_WEIGHTS", "")
		cleanTitle = strings.ReplaceAll(cleanTitle, "_", " ")      // Turns FAST_TRACK into FAST TRACK
		cleanTitle = strings.Join(strings.Fields(cleanTitle), " ") // Cleans up extra spaces

		var roadmapTitle string
		var currentTheme string

		if cleanTitle == "" {
			// If it was just "none" or "custom", it's blank now. Just number it.
			roadmapTitle = fmt.Sprintf("PERSONALIZED ROADMAP %d", i+1)
			currentTheme = "" // FIXED: Leave this blank so it skips the +100 boost loop entirely!
		} else {
			// If there's a theme left (like "FAST TRACK"), append it cleanly.
			roadmapTitle = fmt.Sprintf("PERSONALIZED ROADMAP %d - %s", i+1, cleanTitle)
			currentTheme = cleanTitle
		}

		iterationCourses := make([]RecommendedCourse, len(baseScoredCourses))
		copy(iterationCourses, baseScoredCourses)

		var keywords []string
		themeQuery := strings.ToLower(currentTheme)

		if themeQuery != "" {
			// Loop through all our defined themes in the map (e.g., "game", "code", "business")
			for mapKey, words := range ThemeKeywords {
				mapKeyLower := strings.ToLower(mapKey)

				// If the user's string ("business") matches our map key ("business"), grab the words!
				// This completely ignores plurals and weird string combinations safely.
				if strings.Contains(themeQuery, mapKeyLower) || strings.Contains(mapKeyLower, themeQuery) {
					keywords = words
					break
				}
			}

			// If we successfully grabbed keywords, apply the +100 boost!
			if len(keywords) > 0 {
				for idx, courseRec := range iterationCourses {
					course := courseRec.Course
					courseTitleLower := strings.ToLower(course.CourseName) // Make sure this is the right field!

					for _, word := range keywords {
						wordLower := strings.ToLower(word)
						if strings.Contains(courseTitleLower, wordLower) {
							iterationCourses[idx].FitScore += 100.0
							break
						}
					}
				}
			}
		}

		// --- DETERMINISTIC TIE-BREAKER SORT (FPU VARIANCE FIX) ---
		sort.Slice(iterationCourses, func(a, b int) bool {
			scoreA := iterationCourses[a].FitScore
			scoreB := iterationCourses[b].FitScore

			// If the difference is microscopic, treat it as a tie and alphabetize!
			if math.Abs(scoreA-scoreB) < 0.00001 {
				return iterationCourses[a].Course.CourseCode < iterationCourses[b].Course.CourseCode
			}

			// Otherwise, sort by highest score first
			return scoreA > scoreB
		})

		// FIX: Always shift the courses so sets are unique, even without a theme!
		shiftAmount := i * 3 // Skips the top 3 courses for each new set to force variety
		if len(iterationCourses) > shiftAmount {
			iterationCourses = iterationCourses[shiftAmount:]
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
			schedule   []CourseSchedule
		}
		liveDataCache := make(map[string]apiData)

		sort.Slice(iterationCourses, func(i, j int) bool {
			if iterationCourses[i].FitScore != iterationCourses[j].FitScore {
				return iterationCourses[i].FitScore > iterationCourses[j].FitScore
			}
			// TIE-BREAKER: Alphabetical order
			return iterationCourses[i].Course.CourseCode < iterationCourses[j].Course.CourseCode
		})

		// --- NEW: Track required courses AND keep a waitlist ---
		requiredCount := 0
		var waitlistedElectives []int

		// 1. SELECTION & FILTERING LOOP
		for i, courseRec := range iterationCourses {
			course := courseRec.Course

			if course.CourseCode == "AIC-503" || course.CourseCode == "AIC-602" {
				hasMath211 := false
				for compCode := range studentProfile.Competencies {
					if compCode == "MAT-211" {
						hasMath211 = true
						break
					}
				}
				if !hasMath211 {
					continue
				}
			}

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

			_, startDate, endDate, isAssessmentOnly, isReq, schedule, err := a1ceClient.getCompetencyDetail(course.CourseCode, "Spring 2026", studentProfile.CurriculumVersion)

			if err != nil {
				continue
			}

			var apiGraphics Graphics
			if isAssessmentOnly {
				continue
			}

			// Determine if they chose the bypass weight
			isExplorePassions := strings.Contains(strings.ToUpper(currentTheme), "EXPLORE PASSIONS")

			// --- THE DR. SALLY QUOTA RULE (THE BOUNCER) ---
			// Non-explore_passions: hold back electives until 4 required courses are in.
			// Explore_passions: no minimum required count, but electives are STILL waitlisted
			// so required courses always fill the credit budget first. This prevents high-scoring
			// electives from squeezing out required courses when max credits is low.
			if !isReq && (requiredCount < 4 || isExplorePassions) {
				waitlistedElectives = append(waitlistedElectives, i)
				continue
			}

			// --- SCHEDULE CONFLICT CHECK ---
			// Skip this course if its time slots overlap with any already-selected course.
			conflicting := false
			for _, sel := range selectedCourses {
				if schedulesConflict(schedule, liveDataCache[sel.Course.CourseCode].schedule) {
					conflicting = true
					break
				}
			}
			if conflicting {
				continue
			}

			liveDataCache[course.CourseCode] = apiData{
				startDate:  startDate,
				endDate:    endDate,
				isRequired: isReq,
				graphics:   apiGraphics,
				schedule:   schedule,
			}

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++

			if isReq {
				requiredCount++
			}

			if totalCredits >= targetCredits {
				break
			}
		}

		// --- PHASE 1.1: THE SENIOR WAITLIST RESCUE ---
		// If the student is a senior (or we just ran out of required courses),
		// their schedule might still have room. Let's fill it with the waitlist!
		for _, idx := range waitlistedElectives {
			if totalCredits >= targetCredits {
				break
			}

			courseRec := iterationCourses[idx]
			course := courseRec.Course

			// --- THE MISSING SECURITY GUARD ---
			// Make sure this specific waitlisted elective actually fits in the remaining space!
			if totalCredits+course.CreditHours > targetCredits {
				continue // Too big! Skip it and look for a smaller course.
			}
			// ----------------------------------

			// Fetch the dates AND graphics for the waitlisted course
			_, startDate, endDate, isAssessmentOnly, isReq, schedule, err := a1ceClient.getCompetencyDetail(course.CourseCode, "Spring 2026", studentProfile.CurriculumVersion)

			if err != nil || isAssessmentOnly {
				continue
			}

			var apiGraphics Graphics // Ensures the UI icons don't break

			// Schedule conflict check for waitlisted electives too.
			conflicting := false
			for _, sel := range selectedCourses {
				if schedulesConflict(schedule, liveDataCache[sel.Course.CourseCode].schedule) {
					conflicting = true
					break
				}
			}
			if conflicting {
				continue
			}

			liveDataCache[course.CourseCode] = apiData{
				startDate:  startDate,
				endDate:    endDate,
				isRequired: isReq,
				graphics:   apiGraphics,
				schedule:   schedule,
			}

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++
		}

		// 2. PACKAGING LOOP
		var sumScore, minScore, maxScore, avgScore float64
		var milestones []Milestone

		if len(selectedCourses) > 0 {
			// We will set min/max on the first iteration inside the loop
			firstCourse := true

			// --- DR. SALLY + EAIN FIX: THEME-WEIGHTED PILLAR SORT ---
			// 1. Find the highest FitScore for each prefix group
			pillarMaxScore := make(map[string]float64)
			for _, c := range selectedCourses {
				prefix, _ := getPrefixAndNum(c.Course.CourseCode)
				if c.FitScore > pillarMaxScore[prefix] {
					pillarMaxScore[prefix] = c.FitScore
				}
			}

			// 2. Sort the courses to satisfy both requirements
			for a := 0; a < len(selectedCourses); a++ {
				for b := a + 1; b < len(selectedCourses); b++ {
					prefixA, numA := getPrefixAndNum(selectedCourses[a].Course.CourseCode)
					prefixB, numB := getPrefixAndNum(selectedCourses[b].Course.CourseCode)

					if prefixA != prefixB {
						// DIFFERENT PILLARS: Sort by the block's highest FitScore! (Passes Eain's TC-009)
						if pillarMaxScore[prefixA] < pillarMaxScore[prefixB] {
							selectedCourses[a], selectedCourses[b] = selectedCourses[b], selectedCourses[a]
						} else if pillarMaxScore[prefixA] == pillarMaxScore[prefixB] {
							// If block scores perfectly tie, fall back to alphabetical
							if prefixA > prefixB {
								selectedCourses[a], selectedCourses[b] = selectedCourses[b], selectedCourses[a]
							}
						}
					} else {
						// SAME PILLAR: Sort numerically to keep them neatly grouped (Passes Dr. Sally's rule)
						if numA > numB {
							selectedCourses[a], selectedCourses[b] = selectedCourses[b], selectedCourses[a]
						}
					}
				}
			}
			// --------------------------------------------------------

			for _, c := range selectedCourses {

				// 1. PULL THE ORIGINAL UNIQUE MATH
				finalScore := c.FitScore
				dynamicReason := c.Reason // This holds "Strong Competency Match", etc.

				// --- BUG 1 & 2 FIX: SCORES AND SAFE REASONS ---
				// Grab the specific course's score, NOT the roadmap average
				courseSpecificScore := c.FitScore

				if courseSpecificScore >= 100.0 {
					// Pathfinding prerequisite OR keyword-boosted theme course
					courseSpecificScore = courseSpecificScore - 100.0

					if currentTheme != "" && currentTheme != "None" {
						courseTitleLower := strings.ToLower(c.Course.CourseName)
						isDirectMatch := false
						for _, word := range keywords {
							if strings.Contains(courseTitleLower, strings.ToLower(word)) {
								isDirectMatch = true
								break
							}
						}

						if isDirectMatch {
							dynamicReason = fmt.Sprintf("Exceptional Competency Match (Aligns directly with %s)", currentTheme)
						} else {
							dynamicReason = fmt.Sprintf("Critical Prerequisite (Unlocks advanced %s courses)", currentTheme)
						}
					}
				} else if courseSpecificScore >= 50.0 && currentTheme != "" && currentTheme != "None" {
					// Prefix-boosted theme pillar course — matched by course code, not just title keyword
					courseSpecificScore = courseSpecificScore - 50.0

					// Extract just the base theme name (e.g. "BUSINESS" from "BUSINESS FAST TRACK")
					themeBaseName := currentTheme
					if parts := strings.Fields(currentTheme); len(parts) > 0 {
						themeBaseName = parts[0]
					}

					courseTitleLower := strings.ToLower(c.Course.CourseName)
					isDirectTitleMatch := false
					for _, word := range keywords {
						if strings.Contains(courseTitleLower, strings.ToLower(word)) {
							isDirectTitleMatch = true
							break
						}
					}

					if isDirectTitleMatch {
						dynamicReason = fmt.Sprintf("Strong Theme Alignment (Aligns directly with your %s theme)", themeBaseName)
					} else {
						dynamicReason = fmt.Sprintf("Core %s Course (Belongs to your chosen theme's pillar)", themeBaseName)
					}
				} else if dynamicReason == "" || strings.Contains(dynamicReason, "0.70") {
					// SECURITY FIX: Never expose SubdomainID (UUIDs). Use CourseCode!
					dynamicReason = fmt.Sprintf("Fulfills foundational requirements for %s.", c.Course.CourseCode)
				}

				// 2. NOW CALCULATE THE MATH USING THE TRUE SCORE
				sumScore += finalScore

				if firstCourse {
					minScore = finalScore
					maxScore = finalScore
					firstCourse = false
				} else {
					if finalScore < minScore {
						minScore = finalScore
					}
					if finalScore > maxScore {
						maxScore = finalScore
					}
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

				milestones = append(milestones, Milestone{
					ID:                   c.Course.CourseID,
					TemplateID:           c.Course.TemplateID,
					Title:                c.Course.CourseName,
					CompetencyTitle:      c.Course.CourseName,
					CompetencyCode:       c.Course.CourseCode,
					Credits:              int(c.Course.CreditHours),
					SubdomainTitle:       c.Course.SubdomainID,
					FitScore:             courseSpecificScore,
					Reason:               dynamicReason,
					Graphics:             pillarGraphics,
					StartDate:            data.startDate,
					TargetCompletionDate: data.endDate,
					Required:             data.isRequired,
				})
			}
			avgScore = sumScore / float64(len(selectedCourses))
		}

		// Quality gate: if a real theme was matched (keywords found) but no course in this
		// roadmap reached the theme-boost floor (≥50), all remaining iterations will also
		// be filler — stop early rather than returning low-quality roadmaps.
		// Using len(keywords)>0 instead of currentTheme!="" so weight-only labels like
		// "EXPLORE PASSIONS" or "FAST TRACK" (no real theme) don't trigger this gate.
		if len(keywords) > 0 && maxScore < 50.0 {
			break
		}

		group := MilestoneGroup{
			ID:         "group-auto-gen",
			Title:      "Personalized Recommendations",
			MaxCredits: int(totalCredits),
			Milestones: milestones,
		}

		// --- NEW: DEDUPLICATION FINGERPRINT ---
		var courseIDs []string
		for _, c := range selectedCourses {
			courseIDs = append(courseIDs, c.Course.CourseID)
		}

		// Alphabetize the IDs so order doesn't mess up the fingerprint
		for a := 0; a < len(courseIDs); a++ {
			for b := a + 1; b < len(courseIDs); b++ {
				if courseIDs[a] > courseIDs[b] {
					courseIDs[a], courseIDs[b] = courseIDs[b], courseIDs[a]
				}
			}
		}
		signature := strings.Join(courseIDs, ",")

		if seenSignatures[signature] {
			continue // We already generated this exact roadmap. Skip it!
		}

		// --- THE NEW 0.25 CUTOFF CHECK ---
		// We use 0.25 instead of 0.50 based on Dr. Sally's feedback that 0.50 is too strict
		if avgScore >= minRequiredScore {
			seenSignatures[signature] = true // Mark this fingerprint as "seen"

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

			for _, selectedCourse := range selectedCourses {
				for k := range baseScoredCourses {
					if baseScoredCourses[k].Course.CourseCode == selectedCourse.Course.CourseCode {
						// Apply a 90% penalty to the FitScore
						baseScoredCourses[k].FitScore = baseScoredCourses[k].FitScore * 0.1
					}
				}
			}
		}
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

// parseTimeToMinutes converts "HH:MM" to minutes since midnight for easy comparison.
func parseTimeToMinutes(t string) int {
	parts := strings.SplitN(t, ":", 2)
	if len(parts) != 2 {
		return 0
	}
	h := 0
	m := 0
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return h*60 + m
}

// schedulesConflict returns true if any slot in 'a' overlaps with any slot in 'b'.
// Two slots conflict when they share the same day AND their week ranges overlap
// AND their time ranges overlap. If either course has no schedule data yet
// (API didn't return it), we assume no conflict so we don't over-block.
func schedulesConflict(a, b []CourseSchedule) bool {
	for _, sa := range a {
		for _, sb := range b {
			if sa.Day != sb.Day {
				continue
			}
			// Week range overlap: [sa.StartWeek, sa.EndWeek] ∩ [sb.StartWeek, sb.EndWeek]
			if sa.StartWeek > sb.EndWeek || sb.StartWeek > sa.EndWeek {
				continue
			}
			// Time range overlap: [saStart, saEnd) ∩ [sbStart, sbEnd)
			aStart := parseTimeToMinutes(sa.StartTime)
			aEnd := parseTimeToMinutes(sa.EndTime)
			bStart := parseTimeToMinutes(sb.StartTime)
			bEnd := parseTimeToMinutes(sb.EndTime)
			if aStart < bEnd && bStart < aEnd {
				return true
			}
		}
	}
	return false
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

// --- HELPER: Safe Prefix and Number Extractor ---
func getPrefixAndNum(code string) (string, int) {
	re := regexp.MustCompile(`^([A-Za-z]+)[-\s]*(\d+)`)
	matches := re.FindStringSubmatch(strings.TrimSpace(code))
	if len(matches) >= 3 {
		num, _ := strconv.Atoi(matches[2])
		return strings.ToUpper(matches[1]), num
	}
	return "", 0
}
