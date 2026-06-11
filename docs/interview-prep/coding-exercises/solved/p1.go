package main

type Employee struct {
	Name       string `json:"name"`
	Department string `json:"department"`
	Salary     int    `json:"salary"`
}

func groupByDepartment(employees []Employee) map[string][]Employee {
	grouped := make(map[string][]Employee)
	for _, e := range employees {
		grouped[e.Department] = append(grouped[e.Department], e)
	}

	return grouped
}
