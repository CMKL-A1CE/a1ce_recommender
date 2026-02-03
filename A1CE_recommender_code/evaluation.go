package main

// competency_all_evaluator.go
//
// Implements the Python evaluation pipeline in one file for "all students" mode.
// Logs are now written to logs/evaluation_report.txt
//

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// ---------------------------------------------------------
// Init: redirect all logs to logs/evaluation_report.txt
// ---------------------------------------------------------
func init() {
	// Ensure logs folder exists
	os.MkdirAll("logs", os.ModePerm)

	// Create/open the log file
	logFile, err := os.Create(filepath.Join("logs", "evaluation_report.txt"))
	if err != nil {
		fmt.Println("Failed to create log file:", err)
		return
	}

	// Send all log output to this file
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags)
}

// ---------------------------------------------------------
// Data models used internally
// ---------------------------------------------------------

type TrainRow struct {
	StudentID      string
	CompetencyCode string
	OverallRating  float64
	Grade          float64
}

type CompetencyMeta struct {
	Title    string
	Required int
	Credits  int
}

// ---------------------------------------------------------
// Main exported function
// ---------------------------------------------------------

func EvaluateAllStudentsFromSQLite(dbPath string) error {
	if dbPath == "" {
		dbPath = "a1ce_recommendation.db"
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	competencyMeta, err := loadCompetencyMeta(db)
	if err != nil {
		return fmt.Errorf("load competency data: %w", err)
	}

	sim, err := loadSimilarityMatrix(db)
	if err != nil {
		log.Printf("warning: cannot load similarity matrix: %v (nearest neighbors empty)\n", err)
		sim = make(map[string]map[string]float64)
	}

	prereqs, err := loadPrerequisites(db)
	if err != nil {
		log.Printf("warning: cannot load prerequisites: %v (skipped)\n", err)
		prereqs = make(map[string][]string)
	}

	allTrain, err := loadAllStudentTrain(db)
	if err != nil {
		return fmt.Errorf("load student_train: %w", err)
	}
	if len(allTrain) == 0 {
		return fmt.Errorf("no data in student_train")
	}

	requiredMap := make(map[string]int)
	for code, meta := range competencyMeta {
		requiredMap[code] = meta.Required
	}

	// Group by student
	studentRowsMap := make(map[string][]TrainRow)
	uniqueStudents := []string{}
	for _, r := range allTrain {
		if _, ok := studentRowsMap[r.StudentID]; !ok {
			uniqueStudents = append(uniqueStudents, r.StudentID)
		}
		studentRowsMap[r.StudentID] = append(studentRowsMap[r.StudentID], r)
	}

	sort.Strings(uniqueStudents)
	allRecommendations := make(map[string][]string)

	// ---------------------------------------------------------
	// Generate recommendations
	// ---------------------------------------------------------
	for _, studentID := range uniqueStudents {
		log.Printf("Processing student: %s\n", studentID)

		rows := studentRowsMap[studentID]

		type scored struct {
			Comp  string
			Score float64
		}
		var scoredList []scored

		for _, rr := range rows {
			req := requiredMap[rr.CompetencyCode]
			score := (rr.Grade/4.0)*0.5 + (rr.OverallRating/5.0)*0.3 + float64(req)*0.2
			scoredList = append(scoredList, scored{Comp: rr.CompetencyCode, Score: score})
		}

		topSet := make(map[string]struct{})
		for _, s := range scoredList {
			if s.Score >= 0.8 {
				topSet[s.Comp] = struct{}{}
			}
		}

		if len(topSet) == 0 {
			log.Printf("Student %s: no top competencies (>=0.8)\n", studentID)
			allRecommendations[studentID] = []string{}
			continue
		}

		var topComps []string
		for c := range topSet {
			topComps = append(topComps, c)
		}

		recommendedSet := make(map[string]struct{})
		for _, t := range topComps {
			neighbors := nearestNeighborsFromSim(sim, t, 3)
			for _, n := range neighbors {
				recommendedSet[n] = struct{}{}
			}
		}

		completedSet := topSet
		finalCandidates := []string{}
		for comp := range recommendedSet {
			if reqs, ok := prereqs[comp]; ok {
				missing := false
				for _, pre := range reqs {
					if _, have := completedSet[pre]; !have {
						missing = true
						break
					}
				}
				if missing {
					continue
				}
			}
			finalCandidates = append(finalCandidates, comp)
		}

		completedAll := make(map[string]struct{})
		for _, r := range rows {
			completedAll[r.CompetencyCode] = struct{}{}
		}

		filtered := []string{}
		for _, c := range finalCandidates {
			if _, done := completedAll[c]; !done {
				filtered = append(filtered, c)
			}
		}

		type cm struct {
			Code     string
			Required int
			Credits  int
		}
		var candList []cm
		for _, c := range filtered {
			meta := competencyMeta[c]
			candList = append(candList, cm{Code: c, Required: meta.Required, Credits: meta.Credits})
		}

		sort.Slice(candList, func(i, j int) bool {
			if candList[i].Required != candList[j].Required {
				return candList[i].Required > candList[j].Required
			}
			return candList[i].Credits < candList[j].Credits
		})

		totalCredits := 0
		used := make(map[string]struct{})
		selected := []string{}

		for _, row := range candList {
			if totalCredits+row.Credits <= 60 {
				if _, ok := used[row.Code]; !ok {
					selected = append(selected, row.Code)
					totalCredits += row.Credits
					used[row.Code] = struct{}{}
				}
			}
		}

		allRecommendations[studentID] = selected
	}

	// Write recommendations CSV
	if err := writeRecommendationsCSV("student_recommendations.csv", allRecommendations); err != nil {
		return err
	}
	log.Println("Wrote recommendations to student_recommendations.csv")

	// ---------------------------------------------------------
	// Evaluation
	// ---------------------------------------------------------
	testTruth, err := loadStudentTestTruth(db)
	if err != nil {
		return err
	}

	var accuracies []float64
	for sid, truth := range testTruth {
		rec := allRecommendations[sid]
		correct := intersectionCount(truth, rec)

		acc := 0.0
		if len(truth) > 0 {
			acc = float64(correct) / float64(len(truth))
		}
		accuracies = append(accuracies, acc)

		log.Printf("Student %s: true=%d, recommended=%d, correct=%d, acc=%.2f\n",
			sid, len(truth), len(rec), correct, acc)
	}

	// Average accuracy
    if len(accuracies) == 0 {
        log.Println("No students with truth data found.")
        fmt.Println("Average Recommendation Accuracy: 0.00%")
        fmt.Println("Report written to logs/evaluation_report.txt")
        return nil
    }

    sum := 0.0
    for _, v := range accuracies {
        sum += v
    }
    avg := sum / float64(len(accuracies))
    log.Printf("Average Recommendation Accuracy: %.2f%%\n", avg*100)

    // Print final accuracy ALSO to terminal
    fmt.Printf("Average Recommendation Accuracy: %.2f%%\n", avg*100)

    // Inform user (no blank line)
    fmt.Println("Report written to logs/evaluation_report.txt")

	return nil
}

// ---------------------------------------------------------
// Everything below this line is unchanged
// ---------------------------------------------------------

func loadCompetencyMeta(db *sql.DB) (map[string]CompetencyMeta, error) {
	out := make(map[string]CompetencyMeta)

	rows, err := db.Query(`SELECT competency_code, title, required, credits FROM competency_data`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var code, title sql.NullString
		var requiredRaw interface{}
		var creditsRaw interface{}

		if err := rows.Scan(&code, &title, &requiredRaw, &creditsRaw); err != nil {
			return nil, err
		}
		if !code.Valid {
			continue
		}

		required := 0
		switch v := requiredRaw.(type) {
		case int64:
			required = int(v)
		case bool:
			if v {
				required = 1
			}
		case []byte:
			if strings.TrimSpace(strings.ToLower(string(v))) == "true" {
				required = 1
			}
		case string:
			if strings.TrimSpace(strings.ToLower(v)) == "true" {
				required = 1
			}
		}

		credits := 0
		switch v := creditsRaw.(type) {
		case int64:
			credits = int(v)
		case []byte:
			fmt.Sscanf(string(v), "%d", &credits)
		case string:
			fmt.Sscanf(v, "%d", &credits)
		}

		out[code.String] = CompetencyMeta{
			Title:    title.String,
			Required: required,
			Credits:  credits,
		}
	}
	return out, nil
}

func loadSimilarityMatrix(db *sql.DB) (map[string]map[string]float64, error) {
	out := make(map[string]map[string]float64)

	// Try content_base first
	rows, err := db.Query(`SELECT competency_code_1, competency_code_2, similarity_score_from_content_base FROM Competency_similarity`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var a, b sql.NullString
			var s sql.NullFloat64
			rows.Scan(&a, &b, &s)
			if a.Valid && b.Valid && s.Valid {
				if _, ok := out[a.String]; !ok {
					out[a.String] = make(map[string]float64)
				}
				out[a.String][b.String] = s.Float64
			}
		}
		return out, nil
	}

	// Try topic_modeling next
	rows, err2 := db.Query(`SELECT competency_code_1, competency_code_2, similarity_score_from_topic_modeling FROM Competency_similarity`)
	if err2 == nil {
		defer rows.Close()
		for rows.Next() {
			var a, b sql.NullString
			var s sql.NullFloat64
			rows.Scan(&a, &b, &s)
			if a.Valid && b.Valid && s.Valid {
				if _, ok := out[a.String]; !ok {
					out[a.String] = make(map[string]float64)
				}
				out[a.String][b.String] = s.Float64
			}
		}
		return out, nil
	}

	return nil, fmt.Errorf("similarity matrix not found")
}

func loadPrerequisites(db *sql.DB) (map[string][]string, error) {
	out := make(map[string][]string)

	rows, err := db.Query(`SELECT competency_code, prerequisite_code FROM Competency_prerequisites`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var code, pre sql.NullString
		rows.Scan(&code, &pre)
		if code.Valid && pre.Valid {
			out[code.String] = append(out[code.String], pre.String)
		}
	}
	return out, nil
}

func loadAllStudentTrain(db *sql.DB) ([]TrainRow, error) {
	out := []TrainRow{}

	rows, err := db.Query(`SELECT student_id, competency_code, Overall_rating, Grade FROM student_train`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sid, comp sql.NullString
		var overall, grade sql.NullFloat64
		rows.Scan(&sid, &comp, &overall, &grade)

		if sid.Valid && comp.Valid {
			out = append(out, TrainRow{
				StudentID:      sid.String,
				CompetencyCode: comp.String,
				OverallRating:  overall.Float64,
				Grade:          grade.Float64,
			})
		}
	}
	return out, nil
}

func nearestNeighborsFromSim(sim map[string]map[string]float64, key string, topN int) []string {
	type pair struct {
		Code  string
		Score float64
	}
	list := []pair{}

	if row, ok := sim[key]; ok {
		for k, v := range row {
			if k != key {
				list = append(list, pair{k, v})
			}
		}
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Score > list[j].Score })

	out := []string{}
	for i := 0; i < len(list) && i < topN; i++ {
		out = append(out, list[i].Code)
	}
	return out
}

func writeRecommendationsCSV(filename string, recs map[string][]string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	w.Write([]string{"student_id", "Competency"})

	students := make([]string, 0, len(recs))
	for s := range recs {
		students = append(students, s)
	}
	sort.Strings(students)

	for _, sid := range students {
		comps := recs[sid]
		sort.Strings(comps)
		for _, c := range comps {
			w.Write([]string{sid, c})
		}
	}

	return nil
}

func loadStudentTestTruth(db *sql.DB) (map[string][]string, error) {
	out := make(map[string][]string)

	rows, err := db.Query(`SELECT student_id, competency_code FROM student_test`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sid, comp sql.NullString
		rows.Scan(&sid, &comp)
		if sid.Valid && comp.Valid {
			out[sid.String] = append(out[sid.String], comp.String)
		}
	}

	return out, nil
}

func intersectionCount(a, b []string) int {
	set := make(map[string]struct{})
	for _, x := range a {
		set[strings.TrimSpace(x)] = struct{}{}
	}

	cnt := 0
	for _, y := range b {
		if _, ok := set[strings.TrimSpace(y)]; ok {
			cnt++
		}
	}
	return cnt
}