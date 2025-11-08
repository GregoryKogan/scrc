package kafka

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"scrc/internal/domain/execution"
)

const (
	messageTypeScript = "script"
	messageTypeDone   = "done"
)

type scriptEnvelope struct {
	Type     string           `json:"type"`
	ID       string           `json:"id"`
	Language string           `json:"language"`
	Source   string           `json:"source"`
	Limits   *scriptLimits    `json:"limits,omitempty"`
	Tests    []scriptTestCase `json:"tests,omitempty"`
}

type scriptLimits struct {
	TimeLimitMs      int64 `json:"time_limit_ms"`
	MemoryLimitBytes int64 `json:"memory_limit_bytes"`
}

type scriptTestCase struct {
	Number         int    `json:"number"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
}

type resultEnvelope struct {
	ID         string               `json:"id"`
	Status     execution.Status     `json:"status,omitempty"`
	ExitCode   *int64               `json:"exit_code,omitempty"`
	Stdout     string               `json:"stdout,omitempty"`
	Stderr     string               `json:"stderr,omitempty"`
	DurationMs *int64               `json:"duration_ms,omitempty"`
	Error      string               `json:"error,omitempty"`
	Tests      []testResultEnvelope `json:"tests,omitempty"`
	Timestamp  time.Time            `json:"timestamp"`
}

type testResultEnvelope struct {
	Number         int              `json:"number"`
	Status         execution.Status `json:"status,omitempty"`
	ExitCode       *int64           `json:"exit_code,omitempty"`
	DurationMs     *int64           `json:"duration_ms,omitempty"`
	Stdout         string           `json:"stdout,omitempty"`
	Stderr         string           `json:"stderr,omitempty"`
	Input          string           `json:"input,omitempty"`
	ExpectedOutput string           `json:"expected_output,omitempty"`
	Error          string           `json:"error,omitempty"`
}

func decodeScriptMessage(msg kafkago.Message) (execution.Script, error) {
	var envelope scriptEnvelope
	if err := json.Unmarshal(msg.Value, &envelope); err != nil {
		return execution.Script{}, fmt.Errorf("decode message: %w", err)
	}

	msgType := envelope.Type
	if msgType == "" {
		msgType = messageTypeScript
	}

	switch msgType {
	case messageTypeScript:
		return envelope.toScript(msg)
	case messageTypeDone:
		return execution.Script{}, io.EOF
	default:
		return execution.Script{}, fmt.Errorf("unknown message type %q", msgType)
	}
}

func (e scriptEnvelope) toScript(msg kafkago.Message) (execution.Script, error) {
	if e.Source == "" {
		return execution.Script{}, fmt.Errorf("script message missing source")
	}
	if e.Language == "" {
		return execution.Script{}, fmt.Errorf("script message missing language")
	}

	scriptID := e.ID
	if scriptID == "" {
		scriptID = string(msg.Key)
	}
	if scriptID == "" {
		scriptID = fmt.Sprintf("%s:%d", msg.Topic, msg.Offset)
	}

	return execution.Script{
		ID:       scriptID,
		Language: execution.Language(e.Language),
		Source:   e.Source,
		Limits:   e.toLimits(),
		Tests:    e.toTests(),
	}, nil
}

func (e scriptEnvelope) toLimits() execution.RunLimits {
	if e.Limits == nil {
		return execution.RunLimits{}
	}

	var limits execution.RunLimits
	if e.Limits.TimeLimitMs > 0 {
		limits.TimeLimit = time.Duration(e.Limits.TimeLimitMs) * time.Millisecond
	}
	if e.Limits.MemoryLimitBytes > 0 {
		limits.MemoryLimitBytes = e.Limits.MemoryLimitBytes
	}
	return limits
}

func (e scriptEnvelope) toTests() []execution.TestCase {
	if len(e.Tests) == 0 {
		return nil
	}

	tests := make([]execution.TestCase, len(e.Tests))
	for idx, test := range e.Tests {
		number := test.Number
		if number <= 0 {
			number = idx + 1
		}
		tests[idx] = execution.TestCase{
			Number:         number,
			Input:          test.Input,
			ExpectedOutput: test.ExpectedOutput,
		}
	}
	return tests
}

func encodeRunReport(report execution.RunReport) ([]byte, error) {
	payload, err := json.Marshal(makeResultEnvelope(report))
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return payload, nil
}

func makeResultEnvelope(report execution.RunReport) resultEnvelope {
	var exitCode *int64
	var durationMs *int64
	var stdout string
	var stderr string
	var status execution.Status
	var tests []testResultEnvelope

	if report.Result != nil {
		exit := report.Result.ExitCode
		exitCode = &exit

		dur := report.Result.Duration.Milliseconds()
		durationMs = &dur

		stdout = report.Result.Stdout
		stderr = report.Result.Stderr
		status = report.Result.Status

		if len(report.Result.Tests) > 0 {
			tests = make([]testResultEnvelope, 0, len(report.Result.Tests))
			for _, test := range report.Result.Tests {
				testExit := test.ExitCode
				testDuration := test.Duration.Milliseconds()
				tests = append(tests, testResultEnvelope{
					Number:         test.Case.Number,
					Status:         test.Status,
					ExitCode:       &testExit,
					DurationMs:     &testDuration,
					Stdout:         test.Stdout,
					Stderr:         test.Stderr,
					Input:          test.Case.Input,
					ExpectedOutput: test.Case.ExpectedOutput,
					Error:          test.Error,
				})
			}
		}
	}

	errMsg := ""
	if report.Err != nil {
		errMsg = report.Err.Error()
	}

	return resultEnvelope{
		ID:         report.Script.ID,
		Status:     status,
		ExitCode:   exitCode,
		Stdout:     stdout,
		Stderr:     stderr,
		DurationMs: durationMs,
		Error:      errMsg,
		Tests:      tests,
		Timestamp:  time.Now().UTC(),
	}
}
