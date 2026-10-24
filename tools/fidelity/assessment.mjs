/** Coverage failures are not product defects. Keep both counts visible. */
export function assessDifferences(differences, {reference, actual} = {}) {
  const count = kinds => differences.filter(value => kinds.includes(value.kind)).length;
  const observedProperties = /^(?:font|letterSpacing|textTransform|textDecoration|textShadow|color$|effectiveOpacity$|effectiveBackgroundColor$|background|border|boxShadow$|filter$|backdropFilter$|transform|objectFit$)/;
  const missingObservation = value => value.kind === 'measured-difference' && observedProperties.test(value.property ?? '') &&
    (value.expected === undefined || value.actual === undefined);
  const coverage = {
    declaredUnavailable: {reference: reference?.unavailable?.length ?? 0, actual: actual?.unavailable?.length ?? 0},
    unavailableMeasurements: count(['unavailable-measurement']),
    missingObservedProperties: differences.filter(missingObservation).length,
    invalidMeasurements: count(['invalid-measurement']),
    calibrationFailures: count(['calibration-failure']),
    stateFailures: count(['screen-state', 'fixture-state', 'nondeterministic-extraction', 'font-not-loaded', 'page-readiness', 'native-caret-state']),
  };
  coverage.status = coverage.unavailableMeasurements || coverage.missingObservedProperties || coverage.invalidMeasurements || coverage.calibrationFailures || coverage.stateFailures ||
    coverage.declaredUnavailable.reference || coverage.declaredUnavailable.actual ? 'INCOMPLETE' : 'COMPLETE';
  const findings = {
    measured: differences.filter(value => value.kind === 'measured-difference' && !missingObservation(value)).length, missing: count(['missing-element']), extra: count(['extra-element']),
    ambiguous: count(['ambiguous-match']), cardinality: count(['anchor-cardinality', 'list-cardinality']),
    missingContentSlots: count(['missing-content-slot']),
  };
  const productFindingsReady = coverage.status === 'COMPLETE' && !findings.ambiguous && !findings.cardinality && !findings.missingContentSlots;
  return {coverage, findings, productFindingsReady, measuredVerdict: !productFindingsReady ? 'WITHHELD' : differences.length ? 'FAIL' : 'PASS'};
}
