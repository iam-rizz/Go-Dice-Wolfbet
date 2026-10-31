-- Contoh script dice — martingale dasar
-- Berlaku untuk wolfbet (coin: free / trx / dll).
-- Variabel glob standar: balance, chance, nextbet, previousbet, win, dll.
chance     = 49.5      -- peluang menang %
bethigh    = true
basebet    = 0.01
nextbet    = basebet

function dobet()
  if win then
    nextbet = basebet
  else
    nextbet = previousbet * 2
  end
end
