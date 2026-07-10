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
	MinFitScore      float64                `json:"min_fit_score,omitempty"` //New for weight cut off logic
	// Custom weight values — only used when weight_type = "custom"
	CompetencyWeight float64                `json:"competency_weight,omitempty"`
	InterestWeight   float64                `json:"interest_weight,omitempty"`
	ProgressWeight   float64                `json:"progress_weight,omitempty"`
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
type CompetencyPrerequisiteInfo struct {
	Id                          string `json:"id"`
	dbId                        string
	FocusCompetencyId           string  `json:"focus_competency_id"`
	FocusCompetencyCode         string  `json:"focus_competency_code"`
	FocusCompetencyTitle        string  `json:"focus_competency_title"`
	PrerequisiteCompetencyId    string  `json:"prerequisite_competency_id"`
	PrerequisiteCompetencyCode  string  `json:"prerequisite_competency_code"`
	PrerequisiteCompetencyTitle string  `json:"prerequisite_competency_title"`
	Weight                      float32 `json:"weight"`
	UniversityCode              string  `json:"university_code"`
}

type Course struct {
	CourseID             string                       `json:"course_id"`
	TemplateID           string                       `json:"identity_code,omitempty"` // RENAMED: template_id -> identity_code
	CourseCode           string                       `json:"course_code"`
	CourseName           string                       `json:"course_name"`
	Description          string                       `json:"description,omitempty"`
	CreditHours          float64                      `json:"credit_hours"`
	SubdomainID          string                       `json:"subdomain_id"`
	SubdomainName        string                       `json:"subdomain_name,omitempty"`
	RequiredCompetencies map[string]float64           `json:"required_competencies,omitempty"`
	TeachesCompetencies  []string                     `json:"teaches_competencies,omitempty"`
	Prerequisites        []CompetencyPrerequisiteInfo `json:"prerequisites,omitempty"`
	SemesterOffered      string                       `json:"semester_offered,omitempty"`
	IsCore               bool                         `json:"is_core"`
	IsRequired           bool                         `json:"is_required"`
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
	Title              string              `json:"title"`
	Theme              string              `json:"theme"`
	Courses            []RecommendedCourse `json:"courses"`
	AverageScore       float64             `json:"average_score"`
	MinScore           float64             `json:"min_score"`
	MaxScore           float64             `json:"max_score"`
	TotalCredits       int                 `json:"total_credits"`
	A1CEMilestoneGroup MilestoneGroup      `json:"-"`
}

// CourseSchedule holds one time-slot block for a course.
// Day: "Monday"/"Wednesday"/etc., times: "09:00"/"11:00", weeks: 1-based week numbers.
type CourseSchedule struct {
	Day       string `json:"day"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	StartWeek int    `json:"start_week"`
	EndWeek   int    `json:"end_week"`
}

var ThemeKeywords = map[string][]string{
	// Keywords derived from the actual Spring 2026 catalog course titles.
	// Each keyword must appear in at least one real course title AND must not
	// produce false-positive matches in other theme pillars.
	"code": {
		// SEN courses
		"Programming",           // SEN-102, SEN-103, SEN-109
		"Algorithm",             // SEN-101 "Algorithmic Thinking", SEN-107, SEN-208
		"Database",              // SEN-209 "Designing and Implementing Databases"
		// AIC courses
		"Neural Networks",       // AIC-304
		"Deep Learning",         // AIC-304
		"Machine Learning",      // AIC-507 "Graph-Based Machine Learning"
		"Transformer",           // AIC-503 "Transformer Networks"
		"Generative AI",         // AIC-505
		"Large Language Models", // AIC-506
		"Natural Language Processing", // AIC-602
		"Computer Vision",       // AIC-604
		"Autonomous Agents",     // AIC-603
		// SYS courses
		"Operating Systems",     // SYS-101
		"Cloud Computing",       // SYS-302
		"Parallel Computing",    // SYS-401
		"Big Data",              // SYS-403
	},
	"science": {
		// MAT courses
		"Calculus",              // MAT-100, MAT-103, MAT-105
		"Optimization",          // MAT-104 "Introduction to Optimization"
		"Geometry",              // MAT-106 "Analytical Geometry"
		"Probability",           // MAT-204, MAT-205
		"Statistics",            // MAT-203, MAT-206
		"Signal Processing",     // MAT-202
		"Discrete Math",         // MAT-211, MAT-212, MAT-213, MAT-214
		// SCI courses (from course_identities.json)
		"Biology",               // SCI-101
		"Chemistry",             // SCI-102
		"Quantum",               // SCI-104 Quantum Physics
		"Physics",               // SCI-104, SCI-105, SCI-106, SCI-107
		"Kinematics",            // SCI-105
		"Dynamics",              // SCI-106
		"Thermodynamics",        // SCI-108
		"Electricity",           // SCI-109
		"Magnetism",             // SCI-110
		"Optics",                // SCI-111
		"Mechanics",             // classical mechanics courses
	},
	"games": {
		// HCD courses — all game/media specific
		"Game",                  // HCD-490 "Game Prototype Studio", HCD-541 "Game Engines I"
		"Narrative",             // HCD-533 "Narrative Design"
		"Sound Design",          // HCD-534 (compound phrase, won't match generic "Design")
	},
	"business": {
		// ENI courses
		"Business",              // ENI-202, ENI-204, ENI-304
		"Product Design",        // ENI-103 "Product Design and Development" (compound phrase)
		"Retail",                // ENI-401 "Retail and Services Applications"
		// HAS courses
		"Economics",             // HAS-107 "Principles of Economics"
		// General business terms for future courses
		"Entrepreneurship",
		"Startup",               // SEC-202 "Secure Startup" (acceptable overlap)
	},
}

// ThemePrefixes maps theme names to the CMKL course code prefixes that belong to that theme.
// This enables prefix-based boosting when course titles don't literally match keywords.
// Notes:
//   - SEN (Software Engineering) added to code — all SEN courses are programming/algorithms/databases.
//   - SCI kept for science — physics/chemistry/biology courses (SCI-101 through SCI-111).
//   - SYS removed from games — systems/infrastructure is a code pillar, not game development.
//   - COM (Communication: writing, Thai language) excluded from code intentionally.
var ThemePrefixes = map[string][]string{
	"code":     {"AIC", "SEN", "SYS"},
	"science":  {"MAT", "SCI"},
	"games":    {"AIC", "HCD"},
	"business": {"HAS", "ENI"},
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
	Warning             string        `json:"warning,omitempty"`
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
		Required       bool                         `json:"required"` // <--- NEW: Grabs the required boolean
		Graphics       Graphics                     `json:"graphics"` // <--- NEW: Reuses your existing Graphics struct!
		Prerequisites  []CompetencyPrerequisiteInfo `json:"prerequisites"`
		SemesterDetail struct {
			StartDate      string `json:"start_date"`
			EndDate        string `json:"end_date"`
			AssessmentOnly bool   `json:"assessment_only"` // <--- NEW: Grabs the assessment filter
		} `json:"semester_detail"`
	} `json:"competency"`
}
