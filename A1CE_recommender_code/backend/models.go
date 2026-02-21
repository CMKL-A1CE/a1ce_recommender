package main

import "time"

// Request structures
type RecommendationRequest struct {
	StudentID        string                 `json:"student_id"`
	Semester         string                 `json:"semester"`
	MaxCreditLoad    float64                `json:"max_credit_load"`
	MaxSets          int                    `json:"max_sets"`
	Constraints      *RecommendationFilters `json:"constraints,omitempty"`
	PreviousSemester string                 `json:"previous_semester,omitempty"`
}

type RecommendationFilters struct {
	PreferredSubdomains []string `json:"preferred_subdomains,omitempty"`
	ExcludeCourses      []string `json:"exclude_courses,omitempty"`
	TimePreferences     string   `json:"time_preferences,omitempty"`
}

// Student profile structures
type StudentProfile struct {
	StudentID            string                `json:"student_id"`
	UniversityCode       string                `json:"university_code"`
	CurriculumVersion    int                   `json:"curriculum_version"`
	Competencies         map[string]float64    `json:"competencies"`
	CourseSemesters      map[string]string     `json:"course_semesters"`
	CompletedCourses     []string              `json:"completed_courses"`
	DistributionCredits  map[string]A1CECredit `json:"distribution_credits"`
	RequiredCompetencies []string              `json:"required_competencies"`
	TotalCredits         A1CECredit            `json:"total_credits"`
	InterestWeights      map[string]float64    `json:"interest_weights"`
	MaxCreditLoad        float64               `json:"max_credit_load"`
	Semester             string                `json:"semester"`
}

// Course structures - Internal Logic & Catalog Response
type Course struct {
	CourseID             string             `json:"course_id"`
	TemplateID           string             `json:"identity_code,omitempty"` // RENAMED: template_id -> identity_code
	CourseCode           string             `json:"course_code"`
	CourseName           string             `json:"course_name"`
	Description          string             `json:"description,omitempty"`
	CreditHours          float64            `json:"credit_hours"`
	SubdomainID          string             `json:"subdomain_id"`
	SubdomainName        string             `json:"subdomain_name,omitempty"`
	RequiredCompetencies map[string]float64 `json:"required_competencies,omitempty"`
	TeachesCompetencies  []string           `json:"teaches_competencies,omitempty"`
	Prerequisites        []string           `json:"prerequisites,omitempty"`
	SemesterOffered      string             `json:"semester_offered,omitempty"`
	IsCore               bool               `json:"is_core"`
	IsRequired           bool               `json:"is_required"`
}

// CourseOutput - For Recommendation Response
type CourseOutput struct {
	CourseID             string            `json:"course_id"`
	TemplateID           string            `json:"identity_code,omitempty"` // RENAMED: course_identity -> identity_code
	CourseCode           string            `json:"course_code"`
	CourseName           string            `json:"course_name"`
	Description          string            `json:"description,omitempty"`
	CreditHours          float64           `json:"credit_hours"`
	SubdomainID          string            `json:"subdomain_id"`
	SubdomainName        string            `json:"subdomain_name,omitempty"`
	RequiredCompetencies map[string]string `json:"required_competencies,omitempty"`
	TeachesCompetencies  []string          `json:"teaches_competencies,omitempty"`
	SemesterOffered      string            `json:"semester_offered,omitempty"`
}

// Curriculum requirements
type CurriculumRequirements struct {
	CurriculumVersion        int                `json:"curriculum_version"`
	RequiredCompetencies     []string           `json:"required_competencies"`
	DistributionRequirements map[string]float64 `json:"distribution_requirements"`
	TotalCreditsRequired     float64            `json:"total_credits_required"`
}

// Recommendation output structures
type RecommendedCourse struct {
	Course                 Course       `json:"-"`
	DisplayCourse          CourseOutput `json:"course"`
	FitScore               float64      `json:"fit_score"`
	MatchedCompetencies    []string     `json:"matched_competencies,omitempty"`
	MissingCompetencies    []string     `json:"missing_competencies,omitempty"`
	CompetencyMatchScore   float64      `json:"competency_match_score"`
	InterestAlignmentScore float64      `json:"interest_alignment_score"`
	ProgramProgressScore   float64      `json:"program_progress_score"`
	Reason                 string       `json:"reason"`
}

type RecommendationSet struct {
	StudentID            string                 `json:"student_id"`
	Semester             string                 `json:"semester"`
	RecommendedSet       []RecommendedCourse    `json:"recommended_set"`
	TotalCredits         float64                `json:"total_credits"`
	Metrics              EvaluationMetrics      `json:"metrics"`
	DistributionCoverage map[string]float64     `json:"distribution_coverage"`
	Metadata             RecommendationMetadata `json:"metadata"`
	Status               string                 `json:"status"`
	Warning              string                 `json:"warning,omitempty"`
}

type EvaluationMetrics struct {
	GoodnessScore           float64 `json:"goodness_score"`
	SkillCoveragePercentage float64 `json:"skill_coverage_percentage"`
	PrerequisiteCompliance  float64 `json:"prerequisite_compliance_percentage"`
	ProgramProgressFit      float64 `json:"program_progress_fit"`
}

type RecommendationMetadata struct {
	GenerationTimestamp time.Time `json:"generation_timestamp"`
	AlgorithmVersion    string    `json:"algorithm_version"`
	ProcessingTimeMs    int64     `json:"processing_time_ms"`
}

// A1CE API response structures
type A1CEStudentIdentity struct {
	StudentID         string `json:"id"`
	UniversityCode    string `json:"university_code"`
	CurriculumVersion int    `json:"curriculum_version"`
}

type A1CECompetencyCard struct {
	CompetencyID string  `json:"id"`
	TemplateID   string  `json:"template_id"`
	CourseCode   string  `json:"competency_code"`
	CourseName   string  `json:"title"`
	Grade        float64 `json:"mastery_level"`
	Status       string  `json:"status"`
	Semester     string  `json:"semester_name"`
}

type A1CECredit struct {
	Earned   int `json:"total_earned_credits"`
	Required int `json:"total_required_credits"`
	Working  int `json:"total_working_credits"`
}

type A1CECreditStatus struct {
	DistributionCredits map[string]A1CECredit `json:"distribution_area_credit"`
	TotalCredits        A1CECredit            `json:"overall_credit"`
}

type A1CEGraduationStatus struct {
	RequiredCompetencies []string `json:"required_course_not_taken"`
	A1CECreditStatus
}

type CourseCatalogResponse struct {
	Status            string   `json:"status"`
	Semester          string   `json:"semester"`
	CurriculumVersion int      `json:"curriculum_version"`
	Courses           []Course `json:"courses"`
	TotalCourses      int      `json:"total_courses"`
}
