package main

import (
	"math"
)

// CheckPrerequisites verifies if student meets course requirements
func CheckPrerequisites(course Course, profile *StudentProfile) bool {
	// Check if student has completed prerequisite courses
	for _, prereqID := range course.Prerequisites {
		if !contains(profile.CompletedCourses, prereqID) {
			return false
		}
	}

	// Check competency prerequisites with grade requirements
	for comp, minGrade := range course.RequiredCompetencies {
		studentGrade, hasComp := profile.Competencies[comp]
		if !hasComp || studentGrade < minGrade {
			return false
		}
	}

	return true
}

// CalculateCompetencyMatchScore measures how well student's competencies match course
func CalculateCompetencyMatchScore(course Course, profile *StudentProfile) float64 {
	requiredComps := getMapKeys(course.RequiredCompetencies)
	studentComps := getMapKeys(profile.Competencies)
	taughtComps := course.TeachesCompetencies

	// Matched: Student has these required competencies
	matched := intersection(requiredComps, studentComps)
	// New skills: Course teaches competencies student doesn't have
	newSkills := difference(taughtComps, studentComps)

	// Calculate prerequisite satisfaction rate
	prereqSatisfaction := 1.0
	if len(requiredComps) > 0 {
		prereqSatisfaction = float64(len(matched)) / float64(len(requiredComps))
	}

	// Calculate skill gap filling rate
	skillGapFill := 0.0
	if len(taughtComps) > 0 {
		skillGapFill = float64(len(newSkills)) / float64(len(taughtComps))
	}

	// Grade-level matching
	gradeMatchScore := 0.0
	if len(matched) > 0 {
		for _, comp := range matched {
			requiredGrade := course.RequiredCompetencies[comp]
			studentGrade := profile.Competencies[comp]
			if studentGrade >= requiredGrade {
				gradeMatchScore += 1.0
			} else {
				gradeMatchScore += studentGrade / requiredGrade
			}
		}
		gradeMatchScore /= float64(len(matched))
	} else {
		gradeMatchScore = 1.0
	}

	// Weighted combination
	return 0.4*prereqSatisfaction + 0.3*gradeMatchScore + 0.3*skillGapFill
}

// CalculateInterestScore measures alignment with student's interests
func CalculateInterestScore(course Course, profile *StudentProfile) float64 {
	subdomain := course.SubdomainID

	baseInterest := 0.1 // Default for unexplored areas
	if weight, exists := profile.InterestWeights[subdomain]; exists {
		baseInterest = weight
	}

	// Normalize to 0-1 range
	if baseInterest > 1.0 {
		baseInterest = 1.0
	}

	return baseInterest
}

// CalculateProgramProgressScore measures how much course advances degree completion
func CalculateProgramProgressScore(
	course Course,
	profile *StudentProfile,
	requirements *CurriculumRequirements,
) float64 {
	// Component 1: Required Competency Satisfaction
	missingRequired := difference(requirements.RequiredCompetencies, getMapKeys(profile.Competencies))
	taughtByCourse := course.TeachesCompetencies
	requiredTaught := intersection(missingRequired, taughtByCourse)

	requiredCompScore := 0.0
	if len(missingRequired) > 0 {
		requiredCompScore = float64(len(requiredTaught)) / float64(len(missingRequired))
	}

	// Component 2: Distribution Area Progress
	subdomain := course.SubdomainID
	requiredCredits := 0.0
	if req, exists := requirements.DistributionRequirements[subdomain]; exists {
		requiredCredits = req
	}

	completedCredits := 0.0
	if credits, exists := profile.DistributionCredits[subdomain]; exists {
		completedCredits = float64(credits.Earned)
	}

	distributionScore := 0.0
	if requiredCredits > 0 {
		creditGap := math.Max(0, requiredCredits-completedCredits)
		if creditGap > 0 {
			gapPercentage := creditGap / requiredCredits
			distributionScore = math.Min(1.0, course.CreditHours/creditGap) * gapPercentage
		} else {
			distributionScore = 0.2 // Area already satisfied
		}
	} else {
		distributionScore = 0.3 // Elective
	}

	// Component 3: Overall Degree Progress
	totalProgress := float64(profile.TotalCredits.Earned) / requirements.TotalCreditsRequired
	urgencyMultiplier := 1.0
	if totalProgress < 0.5 {
		urgencyMultiplier = 1.2
	} else if totalProgress < 0.75 {
		urgencyMultiplier = 1.1
	}

	progressScore := (0.5*requiredCompScore +
		0.4*distributionScore +
		0.1*(1.0-totalProgress)) * urgencyMultiplier

	if progressScore > 1.0 {
		progressScore = 1.0
	}

	return progressScore
}

// InferInterestAreas calculates student interests from course history
func InferInterestAreas(completedCourses []string, courseCatalog []Course, competencies map[string]float64) map[string]float64 {
	subdomainCounts := make(map[string]int)
	subdomainPerformance := make(map[string][]float64)

	courseMap := make(map[string]Course)
	for _, course := range courseCatalog {
		courseMap[course.CourseID] = course
	}

	for _, courseID := range completedCourses {
		if course, exists := courseMap[courseID]; exists {
			subdomain := course.SubdomainID
			subdomainCounts[subdomain]++

			if grade, hasGrade := competencies[courseID]; hasGrade {
				subdomainPerformance[subdomain] = append(subdomainPerformance[subdomain], grade)
			}
		}
	}

	interestWeights := make(map[string]float64)
	totalCourses := float64(len(completedCourses))

	if totalCourses == 0 {
		return interestWeights
	}

	for subdomain, count := range subdomainCounts {
		concentrationWeight := float64(count) / totalCourses

		performanceWeight := 0.5 // Default
		if len(subdomainPerformance[subdomain]) > 0 {
			performanceWeight = mean(subdomainPerformance[subdomain]) / 4.0
		}

		interestWeights[subdomain] = 0.6*concentrationWeight + 0.4*performanceWeight
	}

	// Normalize to sum to 1
	totalWeight := 0.0
	for _, weight := range interestWeights {
		totalWeight += weight
	}

	if totalWeight > 0 {
		for subdomain := range interestWeights {
			interestWeights[subdomain] /= totalWeight
		}
	}

	return interestWeights
}

// GetMatchedCompetencies returns competencies student has for this course
func GetMatchedCompetencies(course Course, profile *StudentProfile) []string {
	requiredComps := getMapKeys(course.RequiredCompetencies)
	studentComps := getMapKeys(profile.Competencies)
	return intersection(requiredComps, studentComps)
}

// GetMissingCompetencies returns competencies student lacks for this course
func GetMissingCompetencies(course Course, profile *StudentProfile) []string {
	requiredComps := getMapKeys(course.RequiredCompetencies)
	studentComps := getMapKeys(profile.Competencies)
	return difference(requiredComps, studentComps)
}

// --- SHARED HELPER FUNCTIONS (Used by both recommender and optimizer) ---

// getMapKeys returns keys of a map as a slice
func getMapKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// difference returns elements in 'a' that are not in 'b'
func difference(a, b []string) []string {
	set := make(map[string]bool)
	for _, item := range b {
		set[item] = true
	}
	var result []string
	for _, item := range a {
		if !set[item] {
			result = append(result, item)
		}
	}
	return result
}

// intersection returns elements present in both 'a' and 'b'
func intersection(a, b []string) []string {
	set := make(map[string]bool)
	for _, item := range a {
		set[item] = true
	}
	var result []string
	for _, item := range b {
		if set[item] {
			result = append(result, item)
		}
	}
	return result
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
