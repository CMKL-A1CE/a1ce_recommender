package main

import (
	"math"
	"os"
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
	graphicsMap map[string]Graphics, // <--- ADD THIS NEW PARAMETER!
	baseURL string, // <--- NEW!
	token string, // <--- NEW!
) []CourseSet {

	// 1. Cap the number of roadmaps requested (Max 4)
	if maxSets <= 0 {
		maxSets = 1
	}
	if maxSets > 4 {
		maxSets = 4
	}

	defaultThemes := []string{"code", "science", "games", "business"}
	var roadmaps []CourseSet

	// 2. The Multi-Roadmap Loop
	for i := 0; i < maxSets; i++ {

		currentTheme := preferredTheme
		if currentTheme == "" && i < len(defaultThemes) {
			currentTheme = defaultThemes[i]
		}

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

		// ====================================================================
		// THE FIX: Strict Prerequisite Hard Filter & Clean Selection Loop
		// ====================================================================
		for _, courseRec := range iterationCourses {
			course := courseRec.Course

			// If they haven't met the prerequisites, completely skip this course
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

			selectedCourses = append(selectedCourses, courseRec)
			totalCredits += course.CreditHours
			subdomainCount[course.SubdomainID]++

			if totalCredits >= targetCredits {
				break
			}
		}

		// ====================================================================
		// --- PACKAGE THE ROADMAP (A1CE Nested JSON Format) ---
		// ====================================================================
		var sumScore, minScore, maxScore, avgScore float64
		var milestones []Milestone

		if len(selectedCourses) > 0 {
			minScore = selectedCourses[0].FitScore
			maxScore = selectedCourses[0].FitScore

			for _, c := range selectedCourses {
				sumScore += c.FitScore
				// ... min/max logic ...

				// --- 1. GRAB THE GRAPHICS (Already done!) ---
				prefix := ""
				if len(c.Course.CourseCode) >= 3 {
					prefix = strings.ToUpper(c.Course.CourseCode[:3])
				}
				var pillarGraphics Graphics
				if g, exists := graphicsMap[prefix]; exists {
					pillarGraphics = g
				} else {
					pillarGraphics = Graphics{IconBg: "#f3f4f6", BorderColor: "#9ca3af"}
				}

				// --- 2. GRAB THE DATES (NEW!) ---
				// Call our new helper to fetch the exact dates for this specific course!
				startDate, endDate := fetchCompetencyDates(os.Getenv("M2M_STAGING_API_BASE"), c.Course.CourseCode, "Spring 2026", "YOUR_TOKEN_HERE")
				// Note: You will need to pass the real student token and semester down into this function

				// --- 3. BUILD THE MILESTONE ---
				milestones = append(milestones, Milestone{
					ID:                   c.Course.CourseID,
					TemplateID:           c.Course.TemplateID,
					Title:                c.Course.CourseName,
					CompetencyTitle:      c.Course.CourseName,
					CompetencyCode:       c.Course.CourseCode,
					Credits:              int(c.Course.CreditHours),
					SubdomainTitle:       c.Course.SubdomainID,
					FitScore:             c.FitScore,
					Reason:               c.Reason,
					Graphics:             pillarGraphics,
					StartDate:            startDate, // <--- INJECTED DATE!
					TargetCompletionDate: endDate,   // <--- INJECTED DATE!
				})
			}
			avgScore = sumScore / float64(len(selectedCourses))
		}

		// Wrap the milestones in a MilestoneGroup (as A1CE expects)
		group := MilestoneGroup{
			ID:         "group-auto-gen",
			Title:      "Personalized Recommendations",
			MaxCredits: int(totalCredits),
			Milestones: milestones,
		}

		// Package the final Roadmap struct
		roadmaps = append(roadmaps, CourseSet{
			Theme:              currentTheme,
			Courses:            selectedCourses,
			AverageScore:       avgScore,
			MinScore:           minScore,
			MaxScore:           maxScore,
			TotalCredits:       int(totalCredits),
			A1CEMilestoneGroup: group, // Attach the nested structure!
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
