package postgres

// Export the actual production queries only in the test variant: EXPLAIN must
// fail when job selection regresses, not explain a hand-maintained lookalike.
const CertificateWorkSQLForTest = certificateWorkSQL
const CertificateAuthoritySQLForTest = certificateAuthoritySQL

var RecordDirectAddressTxForTest = recordDirectAddressTx

var WithdrawDirectAddressTxForTest = withdrawDirectAddressTx

var RetireServerTxForTest = retireServerTx
var Install0001TxForTest = install0001Tx

var CustodyReleasedTxForTest = custodyReleasedTx
