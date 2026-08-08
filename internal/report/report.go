// Package report renders a rightsize.Report as either a human table or JSON.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/moveeeax/k8s-rightsize/internal/rightsize"
)

// JSON writes the report as indented JSON.
func JSON(w io.Writer, rep rightsize.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// Table writes an aligned, human-readable report. Rows arrive already sorted by
// reclaimable CPU (biggest offenders first), so the table doubles as a
// worst-waste ranking.
func Table(w io.Writer, rep rightsize.Report) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tCONTAINER\tSTATUS\tCPU req (cur→rec)\tMEM req (cur→rec)\tCPU save")
	for _, r := range rep.Recommendations {
		flag := string(r.Status)
		if r.LowConfidence {
			flag += "?"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s → %s\t%s → %s\t%s\n",
			r.Namespace, r.Container, flag,
			cpu(r.CurCPUReq), cpu(r.CPURequest),
			mem(r.CurMemReq), mem(r.MemRequest),
			signedCPU(r.CPUReqDelta),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nReclaimable: %s CPU, %s memory across %d container(s)\n",
		cpu(rep.CPUSavedCores), mem(rep.MemSavedBytes), len(rep.Recommendations))
	if rep.CPUCostSaved > 0 || rep.MemCostSaved > 0 {
		fmt.Fprintf(w, "Estimated monthly savings: $%.2f\n", rep.CPUCostSaved+rep.MemCostSaved)
	}
	return nil
}

func cpu(cores float64) string {
	if cores == 0 {
		return "-"
	}
	if cores < 1 {
		return fmt.Sprintf("%dm", int(cores*1000+0.5))
	}
	return fmt.Sprintf("%.2f", cores)
}

func signedCPU(cores float64) string {
	if cores <= 0 {
		return "-"
	}
	return cpu(cores)
}

func mem(bytes float64) string {
	if bytes == 0 {
		return "-"
	}
	units := []string{"B", "Ki", "Mi", "Gi", "Ti"}
	v := bytes
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s + units[i]
}
