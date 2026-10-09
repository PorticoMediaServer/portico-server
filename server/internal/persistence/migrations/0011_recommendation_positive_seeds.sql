-- A profile may have a large imported history or many hidden titles whose
-- current taste weight is zero/negative. Seed retrieval needs the most recent
-- positive signals only; filtering the general recent index could walk the
-- entire profile even though the result has a fixed limit.
CREATE INDEX rec_profile_signals_positive_recent ON rec_profile_signals(profile_id,at) WHERE weight>0;
