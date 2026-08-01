package main

import (
	"math"
)

// CalculateCompetencyMatchScore measures how well the student's competencies match a course.
// It reads course.Prerequisites (the real prerequisite list fetched from the M2M API) and
// profile.Competencies (the student's actual recorded grades) — course.RequiredCompetencies
// is never populated by the A1CE client, so it can't be used as a scoring input.
func CalculateCompetencyMatchScore(course Course, profile *StudentProfile) float64 {
	prereqCodes := make([]string, 0, len(course.Prerequisites))
	for _, p := range course.Prerequisites {
		prereqCodes = append(prereqCodes, normalizeCode(p.PrerequisiteCompetencyCode))
	}

	studentGrades := make(map[string]float64, len(profile.Competencies))
	for code, grade := range profile.Competencies {
		studentGrades[normalizeCode(code)] = grade
	}

	// Matched: prerequisites the student has an on-record grade for
	var matched []string
	for _, comp := range prereqCodes {
		if _, ok := studentGrades[comp]; ok {
			matched = append(matched, comp)
		}
	}

	// Calculate prerequisite satisfaction rate
	prereqSatisfaction := 1.0
	if len(prereqCodes) > 0 {
		prereqSatisfaction = float64(len(matched)) / float64(len(prereqCodes))
	}

	// Grade-level matching: how strong were the student's grades in this course's
	// prerequisites? Grades are on a 0-4 scale. Courses with no prerequisites (e.g.
	// intro electives) get a neutral 0.5 instead of a free pass or an unfair penalty.
	gradeMatchScore := 0.5
	if len(matched) > 0 {
		sum := 0.0
		for _, comp := range matched {
			g := studentGrades[comp] / 4.0
			if g > 1.0 {
				g = 1.0
			}
			sum += g
		}
		gradeMatchScore = sum / float64(len(matched))
	}

	// Weighted combination
	return 0.5*prereqSatisfaction + 0.5*gradeMatchScore
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
	// Codes are normalized before comparing since the A1CE API is inconsistent about
	// spacing/casing in competency codes across endpoints.
	missingRequired := normalizeSlice(difference(requirements.RequiredCompetencies, getMapKeys(profile.Competencies)))
	taughtByCourse := normalizeSlice(course.TeachesCompetencies)
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

// normalizeSlice applies normalizeCode (defined in main.go) to every element of a slice.
func normalizeSlice(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = normalizeCode(v)
	}
	return out
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
