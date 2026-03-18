package main

import (
	"math"
	"sort"
	"strings"
	// any other imports you have here
)

// OptimizeCourseSets generates up to 4 thematic roadmaps
func OptimizeCourseSets(
	baseScoredCourses []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
	maxCreditLoad float64,
	maxSets int,
	preferredTheme string,
) []CourseSet {

	// 1. Cap the number of roadmaps requested (Max 4)
	if maxSets <= 0 { maxSets = 1 }
	if maxSets > 4 { maxSets = 4 }

	defaultThemes := []string{"code", "science", "games", "business"}
	var roadmaps []CourseSet

	// 2. The Multi-Roadmap Loop
	for i := 0; i < maxSets; i++ {
		
		// Determine the theme for this specific roadmap iteration
		currentTheme := preferredTheme
		if currentTheme == "" && i < len(defaultThemes) {
			currentTheme = defaultThemes[i]
		}

		// --- THEME SCORING & SORTING ---
		// Create a fresh copy of the courses for this loop iteration 
		// so we don't permanently mess up the base scores!
		iterationCourses := make([]RecommendedCourse, len(baseScoredCourses))
		copy(iterationCourses, baseScoredCourses)

		// Apply the Theme Bonus
		if currentTheme != "" {
			keywords := ThemeKeywords[strings.ToLower(currentTheme)]
			for idx, courseRec := range iterationCourses {
				course := courseRec.Course
				// Check if CourseCode or SubdomainID matches the theme keywords
				for _, word := range keywords {
					if strings.Contains(strings.ToLower(course.CourseCode), strings.ToLower(word)) ||
					   strings.Contains(strings.ToLower(course.SubdomainID), strings.ToLower(word)) {
						// Give a massive boost to force it to the top of the selection pool
						iterationCourses[idx].FitScore += 100.0
						break
					}
				}
			}
		}

		// Re-sort the copied list based on the new themed scores (highest score first)
		sort.Slice(iterationCourses, func(a, b int) bool {
    	return iterationCourses[a].FitScore > iterationCourses[b].FitScore 
		})
		// --- VARIETY GENERATOR ---
		// To ensure roadmaps are actually different if the theme is the same, skip the top 'i' courses
		if len(iterationCourses) > i && preferredTheme != "" {
			iterationCourses = iterationCourses[i:] // Slice off the top 'i' elements
		}

		// ====================================================================
		// --- YOUR ORIGINAL SELECTION LOGIC (Unchanged, just uses iterationCourses) ---
		// ====================================================================
		var selectedCourses []RecommendedCourse
		totalCredits := 0.0
		targetCredits := maxCreditLoad 

		subdomainCount := make(map[string]int)
		maxPerSubdomain := 10

		graduationReqMap := make(map[string]bool)
		for _, req := range requirements.RequiredCompetencies {
			graduationReqMap[req] = true
		}

		priorityCount := 0
		targetPriorityCount := 3

		isGraduationRequirement := func(c Course) bool { // Assumes 'Course' is your struct name
			if graduationReqMap[c.CourseCode] { return true }
			if graduationReqMap[c.CourseID] { return true }
			for _, taught := range c.TeachesCompetencies {
				if graduationReqMap[taught] { return true }
			}
			return false
		}

		// Phase 1: Priority Pass
		for _, courseRec := range iterationCourses {
			if priorityCount >= targetPriorityCount { break }

			course := courseRec.Course

			if !isGraduationRequirement(course) { continue }
			if totalCredits+course.CreditHours > targetCredits { continue }
			if containsRecommendedCourse(selectedCourses, courseRec) { continue }

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++
			priorityCount++
		}

		// Phase 2: Fill the rest
		for _, courseRec := range iterationCourses {
			course := courseRec.Course

			if containsRecommendedCourse(selectedCourses, courseRec) { continue }
			if totalCredits+course.CreditHours > targetCredits { continue }
			if subdomainCount[course.SubdomainID] >= maxPerSubdomain { continue }

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++

			if totalCredits >= targetCredits { break }
		}
		// ====================================================================

		// --- PACKAGE THE ROADMAP ---
		var roadmapScore float64
		for _, c := range selectedCourses {
			roadmapScore += c.FitScore 
		}

		roadmaps = append(roadmaps, CourseSet{
			Theme:        currentTheme,
			Courses:      selectedCourses,
			TotalScore:   roadmapScore,
			TotalCredits: totalCredits,
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