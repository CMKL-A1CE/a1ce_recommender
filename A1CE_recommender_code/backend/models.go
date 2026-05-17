package main

import (
	"time"
)

// Request structures
type RecommendationRequest struct {
	StudentID        string                 `json:"student_id"`
	Semester         string                 `json:"semester"`
	MaxCreditLoad    float64                `json:"max_credit_load"`
	MaxSets          int                    `json:"max_sets"`
	Constraints      *RecommendationFilters `json:"constraints,omitempty"`
	PreviousSemester string                 `json:"previous_semester,omitempty"`
	PreferredTheme   string                 `json:"preferred_theme"`
	WeightType       string                 `json:"weight_type,omitempty"`
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

type CourseSet struct {
	Theme              string              `json:"theme"`
	Courses            []RecommendedCourse `json:"courses"`
	AverageScore       float64             `json:"average_score"`
	MinScore           float64             `json:"min_score"`
	MaxScore           float64             `json:"max_score"`
	TotalCredits       int                 `json:"total_credits"`
	A1CEMilestoneGroup MilestoneGroup      `json:"-"`
}

var ThemeKeywords = map[string][]string{
	"code":     {"Programming", "Software", "Data", "Algorithm", "Computer", "Network"},
	"science":  {"Physics", "Math", "Biology", "Chemistry", "Calculus", "Science"},
	"games":    {"Game", "Interactive", "Graphics", "Strategy", "Survival", "Engine"},
	"business": {"Business", "Management", "Economics", "Marketing", "Entrepreneurship"},
}

type RecommendationResponse struct {
	StudentID   string         `json:"student_id"`
	Semester    string         `json:"semester"`
	Roadmaps    []CourseSet    `json:"roadmaps"`
	Status      string         `json:"status"`
	Warning     string         `json:"warning"`
	WeightsUsed ScoringWeights `json:"weights_used"`
}

// Holds the current live weights for the algorithm
type ScoringWeights struct {
	Competency float64 `json:"competency_weight"`
	Interest   float64 `json:"interest_weight"`
	Progress   float64 `json:"progress_weight"`
}

// The response when someone successfully updates the weights
type WeightUpdateResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Weights ScoringWeights `json:"current_weights"`
}

// --- A1CE Official API Response Schemas ---

type A1CEResponse struct {
	RecommendedRoadmaps []A1CERoadmap `json:"recommended_roadmaps"`
	Status              string        `json:"status"`
}

type A1CERoadmap struct {
	ID                string           `json:"id"`
	Title             string           `json:"title"`
	Year              int              `json:"year"`
	Semester          string           `json:"semester"`
	Credits           int              `json:"credits"`
	AverageScore      float64          `json:"average_score"` // Our custom field
	MinScore          float64          `json:"min_score"`     // Our custom field
	MaxScore          float64          `json:"max_score"`     // Our custom field
	TermOrdinal       int              `json:"term_ordinal"`
	CurriculumVersion int              `json:"curriculum_version"`
	MilestoneGroups   []MilestoneGroup `json:"milestone_groups"`
	UniversityCode    string           `json:"university_code"`
	StudyTrack        int              `json:"study_track"`
}

type MilestoneGroup struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	MaxCredits int         `json:"max_credits"`
	Milestones []Milestone `json:"milestones"`
}

type Milestone struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	TemplateID           string   `json:"template_id"`
	TemplatePillarID     string   `json:"template_pillar_id"`
	PillarTitle          string   `json:"pillar_title"`
	SubdomainTitle       string   `json:"subdomain_title"`
	CompetencyTitle      string   `json:"competency_title"`
	CompetencyCode       string   `json:"competency_code"`
	Required             bool     `json:"required"`
	StartDate            string   `json:"start_date"`
	TargetCompletionDate string   `json:"target_completion_date"`
	Credits              int      `json:"credits"`
	IsAttendanceRequired bool     `json:"is_attendance_required"`
	Skills               []Skill  `json:"skills"`
	Graphics             Graphics `json:"graphics"`
	FitScore             float64  `json:"fit_score"` // Our custom field
	Reason               string   `json:"reason"`    // Our custom field
}

type Skill struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SkillCode   string `json:"skill_code"`
	Required    bool   `json:"required"`
}

type Graphics struct {
	IconBg      string `json:"iconbg"`
	BorderColor string `json:"bordercolor"`
	Icon        string `json:"icon"`
	IconShadow  string `json:"iconshadow"`
}

type PillarInfo struct {
	Id             string        `json:"id"`
	DbId           string        `json:"-"` // Ignore for JSON parsing
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	PillarCode     string        `json:"code"`
	PillarPrefix   string        `json:"prefix"`
	IsCore         bool          `json:"is_core"`
	Ordinal        int           `json:"ordinal"`
	Subdomains     []interface{} `json:"subdomains"` // Catch-all for subdomains
	PillarGraphics Graphics      `json:"graphics"`   // Reusing your existing Graphics struct!
}

// --- API Response Schemas for fetching Dates ---
type CompetencyDetailResponse struct {
	Competency struct {
		SemesterDetail struct {
			StartDate string `json:"start_date"`
			EndDate   string `json:"end_date"`
		} `json:"semester_detail"`
	} `json:"competency"`
}
