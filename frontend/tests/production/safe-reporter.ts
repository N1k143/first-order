import type {
  FullConfig,
  FullResult,
  Reporter,
  Suite,
  TestCase,
  TestResult,
} from '@playwright/test/reporter';

// No default call logs, errors, attachments, screenshots or traces: fill() error
// messages can contain a password. Step names and cleanup IDs live in a private manifest.
export default class SafeReporter implements Reporter {
  printsToStdio() {
    return true;
  }
  onBegin(_config: FullConfig, suite: Suite) {
    process.stdout.write(JSON.stringify({ tests_discovered: suite.allTests().length }) + '\n');
  }
  onTestEnd(test: TestCase, result: TestResult) {
    process.stdout.write(
      JSON.stringify({ test: test.title, status: result.status, duration_ms: result.duration }) +
        '\n',
    );
  }
  onError() {
    process.stderr.write('{"error":"E2E_SETUP_OR_WORKER_ERROR"}\n');
  }
  onStdOut() {}
  onStdErr() {}
  onEnd(result: FullResult) {
    process.stdout.write(JSON.stringify({ result: result.status }) + '\n');
  }
}
