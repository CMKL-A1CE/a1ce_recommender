package main

import (
	"fmt"
	"sort"
	"time"
)

type RecommenderService struct {
	a1ceClient *A1CEClient
}

func NewRecommenderService() *RecommenderService {
	return &RecommenderService{
		a1ceClient: NewA1CEClient(),
	}
}

// GenerateRecommendations is the main service method
func (s *RecommenderService) GenerateRecommendations(req *RecommendationRequest) (*RecommendationSet, error) {
	startTime := time.Now()

	// Step 1: Fetch student profile
	studentProfile, err := s.a1ceClient.GetStudentProfile(req.StudentID)
	fmt.Println("\tjwt token: ", s.a1ceClient.JWTToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch student profile: %w", err)
	}
	studentProfile.Semester = req.Semester
	studentProfile.MaxCreditLoad = req.MaxCreditLoad

	// Step 2: Fetch course catalog
	catalog, err := s.a1ceClient.GetCourseCatalog(req.Semester, studentProfile.CurriculumVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch course catalog: %w", err)
	}

	// Step 3: Fetch curriculum requirements
	requirements := &CurriculumRequirements{
		CurriculumVersion:    studentProfile.CurriculumVersion,
		RequiredCompetencies: studentProfile.RequiredCompetencies,
		DistributionRequirements: map[string]float64{
			"AI":   36,
			"SE":   24,
			"Math": 12,
		},
		TotalCreditsRequired: 120,
	}

	// Step 4: Infer interest areas
	studentProfile.InterestWeights = InferInterestAreas(
		studentProfile.CompletedCourses,
		catalog.Courses,
		studentProfile.Competencies,
	)

	// Step 5: Generate candidate courses (filter)
	candidateCourses := s.filterCandidateCourses(catalog.Courses, studentProfile, req.Constraints)

	// Step 6: Score each candidate course
	scoredCourses := s.scoreCourses(candidateCourses, studentProfile, requirements)

	// Step 7: Optimize course set selection
	recommendedSet := OptimizeCourseSet(scoredCourses, studentProfile, requirements, req.MaxCreditLoad)

	// Step 8: Evaluate recommendation quality
	metrics := EvaluateRecommendationSet(recommendedSet, studentProfile, requirements)

	// Step 9: Build final response
	result := &RecommendationSet{
		StudentID:            req.StudentID,
		Semester:             req.Semester,
		RecommendedSet:       recommendedSet,
		TotalCredits:         calculateTotalCredits(recommendedSet),
		Metrics:              *metrics,
		DistributionCoverage: calculateDistributionCoverage(recommendedSet),
		Metadata: RecommendationMetadata{
			GenerationTimestamp: time.Now(),
			AlgorithmVersion:    "1.0",
			ProcessingTimeMs:    time.Since(startTime).Milliseconds(),
		},
		Status: "success",
	}

	return result, nil
}

// filterCandidateCourses removes ineligible courses
func (s *RecommenderService) filterCandidateCourses(
	allCourses []Course,
	profile *StudentProfile,
	constraints *RecommendationFilters,
) []Course {
	var candidates []Course

	for _, course := range allCourses {
		// Filter 1: Already completed
		if contains(profile.CompletedCourses, course.CourseID) {
			continue
		}

		// Filter 2: Prerequisites not satisfied
		if !CheckPrerequisites(course, profile) {
			continue
		}

		// Filter 3: User constraints - excluded courses
		if constraints != nil && contains(constraints.ExcludeCourses, course.CourseID) {
			continue
		}

		candidates = append(candidates, course)
	}

	return candidates
}

// scoreCourses calculates fit scores for all candidate courses
func (s *RecommenderService) scoreCourses(
	courses []Course,
	profile *StudentProfile,
	requirements *CurriculumRequirements,
) []RecommendedCourse {
	var scored []RecommendedCourse

	for _, course := range courses {
		compScore := CalculateCompetencyMatchScore(course, profile)
		interestScore := CalculateInterestScore(course, profile)
		progressScore := CalculateProgramProgressScore(course, profile, requirements)

		fitScore := 0.4*compScore + 0.3*interestScore + 0.3*progressScore

		recommended := RecommendedCourse{
			Course:                 course,
			FitScore:               fitScore,
			CompetencyMatchScore:   compScore,
			InterestAlignmentScore: interestScore,
			ProgramProgressScore:   progressScore,
			MatchedCompetencies:    GetMatchedCompetencies(course, profile),
			MissingCompetencies:    GetMissingCompetencies(course, profile),
			Reason:                 generateReason(course, fitScore, progressScore, interestScore),
		}

		scored = append(scored, recommended)
	}

	// Sort by fit score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].FitScore > scored[j].FitScore
	})

	return scored
}

func generateReason(course Course, fitScore, progressScore, interestScore float64) string {
	if progressScore > 0.7 {
		return fmt.Sprintf("Satisfies important graduation requirements in %s", course.SubdomainName)
	}
	if interestScore > 0.7 {
		return fmt.Sprintf("Strongly aligns with your interests in %s", course.SubdomainName)
	}
	if fitScore > 0.8 {
		return "Excellent fit based on your competencies and academic goals"
	}
	return fmt.Sprintf("Good course option in %s that advances your degree progress", course.SubdomainName)
}

func calculateTotalCredits(courses []RecommendedCourse) float64 {
	total := 0.0
	for _, c := range courses {
		total += c.Course.CreditHours
	}
	return total
}

func calculateDistributionCoverage(courses []RecommendedCourse) map[string]float64 {
	coverage := make(map[string]float64)
	for _, c := range courses {
		coverage[c.Course.SubdomainID] += c.Course.CreditHours
	}
	return coverage
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
