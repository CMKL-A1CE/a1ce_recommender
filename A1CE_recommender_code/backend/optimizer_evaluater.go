package main

import "math"

// OptimizeCourseSet selects optimal combination of courses for the semester
func OptimizeCourseSet(
	scoredCourses []RecommendedCourse,
	studentProfile *StudentProfile,
	requirements *CurriculumRequirements,
	maxCreditLoad float64,
) []RecommendedCourse {
	var selectedCourses []RecommendedCourse
	totalCredits := 0.0

	// UPDATED: Allow maxCreditLoad to exceed 60 if requested
	// absoluteMax := 60.0
	targetCredits := maxCreditLoad // math.Min(maxCreditLoad, absoluteMax) removed

	subdomainCount := make(map[string]int)
	maxPerSubdomain := 10

	// 1. Identify Graduation Requirements
	graduationReqMap := make(map[string]bool)
	for _, req := range requirements.RequiredCompetencies {
		graduationReqMap[req] = true
	}

	// 2. Priority Selection: Pick up to 3 distinct Graduation Requirements first
	priorityCount := 0
	targetPriorityCount := 3

	// Helper to check if course satisfies a missing graduation requirement
	isGraduationRequirement := func(c Course) bool {
		if graduationReqMap[c.CourseCode] {
			return true
		}
		if graduationReqMap[c.CourseID] {
			return true
		}
		for _, taught := range c.TeachesCompetencies {
			if graduationReqMap[taught] {
				return true
			}
		}
		return false
	}

	// Phase 1: Priority Pass
	for _, courseRec := range scoredCourses {
		if priorityCount >= targetPriorityCount {
			break
		}

		course := courseRec.Course

		if !isGraduationRequirement(course) {
			continue
		}

		if totalCredits+course.CreditHours > targetCredits {
			continue
		}
		if containsRecommendedCourse(selectedCourses, courseRec) {
			continue
		}

		selectedCourses = append(selectedCourses, courseRec)
		totalCredits += course.CreditHours
		subdomainCount[course.SubdomainID]++
		priorityCount++
	}

	// Phase 2: Fill the rest
	for _, courseRec := range scoredCourses {
		course := courseRec.Course

		if containsRecommendedCourse(selectedCourses, courseRec) {
			continue
		}

		if totalCredits+course.CreditHours > targetCredits {
			continue
		}
		if subdomainCount[course.SubdomainID] >= maxPerSubdomain {
			continue
		}

		selectedCourses = append(selectedCourses, courseRec)
		totalCredits += course.CreditHours
		subdomainCount[course.SubdomainID]++

		// UPDATED: Relax the break condition slightly to allow filling up to exact target
		if totalCredits >= targetCredits {
			break
		}
	}

	return selectedCourses
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
