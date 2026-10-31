-- mini version of the user's "1/5jam ls 37" strategy that reads state
-- globals at top level. Kept deterministic (no recovery mode, no vault,
-- no stop conditions) so the bridge test can assert exact bet sizes.
chance   = 22
softmult = 1.25
MINBET   = 0.0000001
BASEDIV  = 5 / 0.000015

basediv = BASEDIV
base    = math.max(balance / basediv, MINBET)
nextbet = base

function dobet()
  if win then
    nextbet = base
  else
    nextbet = math.max(base, previousbet * softmult)
  end
end
