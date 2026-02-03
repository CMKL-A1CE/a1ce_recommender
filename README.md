# A1CE-RecSys
## How to Run and Test the API

### 1. Prerequisites
Make sure your working directory is the **A1CE_recommender** folder before running any commands.
Have Go and Python installed on your computer.

### 2. Start the Backend
Open **Terminal 1** and run:
```bash
go run .
```

### 3. Start the Test Interface
Open **Terminal 2** and run:
```bash
python3 -m http.server 3000
```

### 4. Access the Test Page
Open a browser and navigate to:
```
http://localhost:3000/A1CE_recommender/test_interface.html
```

### 5. Retrieve Data From A1CE
1. Go to the A1CE website.
2. Right–click and select Inspect to open developer tools.
3. Locate and copy your:
    - ID
    - JWT token
4. Enter these values into the test interface to run API calls.

## How to run evaluation
### 1. Prerequisites
Make sure your working directory is the **A1CE_recommender** folder before running any commands.
Have Go and Python installed on your computer.

### 2. Start the evaluation process
Open **Terminal** and run:
```bash
go run . eval
```

### 3. Check the output
- After it finishes executing, you can see a message showing the overall accuracy and that a report was successfully written to a text file.
- You can view more detailed evaluation results inside the **logs/evaluation_report.txt** file, which contains per-student recommendation accuracy.
- You can also review all model-generated recommendations for each student in the **student_recommendations.csv** file. Each recommendation is listed using its competency code.
