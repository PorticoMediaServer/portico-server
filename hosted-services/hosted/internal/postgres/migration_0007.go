package postgres

// 0007: CGNAT prevalence is counted in memory now (A49); the singleton row
// that serialized every route observation goes.
const migration0007 = `
DROP TABLE cgnat_aggregate;
`
