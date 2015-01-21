package main

import (
	"bytes"
	"fmt"
	"github.com/getgauge/gauge/gauge_messages"
	"sort"
	"strings"
)

const (
	TABLE_LEFT_SPACING = 5
)

func getRepeatedChars(character string, repeatCount int) string {
	formatted := ""
	for i := 0; i < repeatCount; i++ {
		formatted = fmt.Sprintf("%s%s", formatted, character)
	}
	return formatted
}

func formatSpecHeading(specHeading string) string {
	return formatHeading(specHeading, "=")
}

func formatScenarioHeading(scenarioHeading string) string {
	return fmt.Sprintf("%s", formatHeading(scenarioHeading, "-"))
}

func formatStep(step *step) string {
	text := step.value
	paramCount := strings.Count(text, PARAMETER_PLACEHOLDER)
	for i := 0; i < paramCount; i++ {
		argument := step.args[i]
		formattedArg := ""
		if argument.argType == tableArg {
			formattedTable := formatTable(&argument.table)
			formattedArg = fmt.Sprintf("\n%s", formattedTable)
		} else if argument.argType == dynamic {
			formattedArg = fmt.Sprintf("<%s>", getUnescapedString(argument.value))
		} else if argument.argType == specialString || argument.argType == specialTable {
			formattedArg = fmt.Sprintf("<%s>", getUnescapedString(argument.name))
		} else {
			formattedArg = fmt.Sprintf("\"%s\"", getUnescapedString(argument.value))
		}
		text = strings.Replace(text, PARAMETER_PLACEHOLDER, formattedArg, 1)
	}
	stepText := ""
	if strings.HasSuffix(text, "\n") {
		stepText = fmt.Sprintf("* %s", text)
	} else {
		stepText = fmt.Sprintf("* %s\n", text)
	}
	return stepText
}

func formatConcept(protoConcept *gauge_messages.ProtoConcept) string {
	conceptText := "# "
	for _, fragment := range protoConcept.ConceptStep.GetFragments() {
		if fragment.GetFragmentType() == gauge_messages.Fragment_Text {
			conceptText = conceptText + fragment.GetText()
		} else if fragment.GetFragmentType() == gauge_messages.Fragment_Parameter {
			if fragment.GetParameter().GetParameterType() == (gauge_messages.Parameter_Table | gauge_messages.Parameter_Special_Table) {
				conceptText += "\n" + formatTable(tableFrom(fragment.GetParameter().GetTable()))
			} else {
				conceptText = conceptText + "\"" + fragment.GetParameter().GetValue() + "\""
			}
		}
	}
	return conceptText
}

func formatHeading(heading, headingChar string) string {
	trimmedHeading := strings.TrimSpace(heading)
	length := len(trimmedHeading)
	return fmt.Sprintf("%s\n%s\n", trimmedHeading, getRepeatedChars(headingChar, length))
}

func formatTable(table *table) string {
	columnToWidthMap := make(map[int]int)
	for i, header := range table.headers {
		//table.get(header) returns a list of cells in that particular column
		cells := table.get(header)
		columnToWidthMap[i] = findLongestCellWidth(cells, len(header))
	}

	var tableStringBuffer bytes.Buffer
	tableStringBuffer.WriteString(fmt.Sprintf("%s|", getRepeatedChars(" ", TABLE_LEFT_SPACING)))
	for i, header := range table.headers {
		width := columnToWidthMap[i]
		tableStringBuffer.WriteString(fmt.Sprintf("%s|", addPaddingToCell(header, width)))
	}

	tableStringBuffer.WriteString("\n")
	tableStringBuffer.WriteString(fmt.Sprintf("%s|", getRepeatedChars(" ", TABLE_LEFT_SPACING)))
	for i, _ := range table.headers {
		width := columnToWidthMap[i]
		cell := getRepeatedChars("-", width)
		tableStringBuffer.WriteString(fmt.Sprintf("%s|", addPaddingToCell(cell, width)))
	}

	tableStringBuffer.WriteString("\n")
	for _, row := range table.getRows() {
		tableStringBuffer.WriteString(fmt.Sprintf("%s|", getRepeatedChars(" ", TABLE_LEFT_SPACING)))
		for i, cell := range row {
			width := columnToWidthMap[i]
			tableStringBuffer.WriteString(fmt.Sprintf("%s|", addPaddingToCell(cell, width)))
		}
		tableStringBuffer.WriteString("\n")
	}

	return string(tableStringBuffer.Bytes())
}

func addPaddingToCell(cellValue string, width int) string {
	padding := getRepeatedChars(" ", width-len(cellValue))
	return fmt.Sprintf("%s%s", cellValue, padding)
}

func findLongestCellWidth(columnCells []tableCell, minValue int) int {
	longestLength := minValue
	for _, cellValue := range columnCells {
		cellValueLen := len(cellValue.value)
		if cellValueLen > longestLength {
			longestLength = cellValueLen
		}
	}
	return longestLength
}

func formatItem(item item) string {
	switch item.kind() {
	case commentKind:
		comment := item.(*comment)
		if comment.value == "\n" {
			return comment.value
		}
		return fmt.Sprintf("%s\n", comment.value)
	case stepKind:
		step := item.(*step)
		return formatStep(step)
	case tableKind:
		table := item.(*table)
		return formatTable(table)
	case scenarioKind:
		scenario := item.(*scenario)
		var b bytes.Buffer
		b.WriteString(formatScenarioHeading(scenario.heading.value))
		b.WriteString(formatTags(scenario.tags))
		b.WriteString(formatItems(scenario.items))
		return string(b.Bytes())
	}
	return ""
}

func formatTags(tags *tags) string {
	if tags == nil || len(tags.values) == 0 {
		return ""
	}
	var b bytes.Buffer
	b.WriteString("\ntags: ")
	for i, tag := range tags.values {
		b.WriteString(tag)
		if (i + 1) != len(tags.values) {
			b.WriteString(", ")
		}
	}
	b.WriteString("\n\n")
	return string(b.Bytes())
}

func formatItems(items []item) string {
	var result bytes.Buffer
	for _, item := range items {
		result.WriteString(formatItem(item))
	}
	return string(result.Bytes())
}

func formatSpecification(specification *specification) string {
	var formattedText bytes.Buffer
	formattedText.WriteString(formatSpecHeading(specification.heading.value))
	formattedText.WriteString(formatTags(specification.tags))
	formattedText.WriteString(formatItems(specification.items))
	return string(formattedText.Bytes())
}

type ByLineNo []*concept

func (s ByLineNo) Len() int {
	return len(s)
}

func (s ByLineNo) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByLineNo) Less(i, j int) bool {
	return s[i].conceptStep.lineNo < s[j].conceptStep.lineNo
}

func sortConcepts(conceptDictionary *conceptDictionary, conceptMap map[string]string) []*concept {
	concepts := make([]*concept, 0)
	for _, concept := range conceptDictionary.conceptsMap {
		conceptMap[concept.fileName] = ""
		concepts = append(concepts, concept)
	}
	sort.Sort(ByLineNo(concepts))
	return concepts
}

func formatConceptSteps(conceptDictionary *conceptDictionary, conceptMap map[string]string, concept *concept) {
	conceptMap[concept.fileName] += strings.TrimSpace(strings.Replace(formatItem(concept.conceptStep), "*", "#", 1)) + "\n"
	for i := 1; i < len(concept.conceptStep.items); i++ {
		conceptMap[concept.fileName] += formatItem(concept.conceptStep.items[i])
	}
}

func formatConcepts(conceptDictionary *conceptDictionary) map[string]string {
	conceptMap := make(map[string]string)
	for _, concept := range sortConcepts(conceptDictionary, conceptMap) {
		for _, comment := range concept.conceptStep.preComments {
			conceptMap[concept.fileName] += formatItem(comment)
		}
		formatConceptSteps(conceptDictionary, conceptMap, concept)
	}
	return conceptMap
}