<?php
// Returns the Drobo report as JSON for the Main panel.
// ?fresh=1 skips the 20 second cache (the Refresh button).
header('Content-Type: application/json');
header('Cache-Control: no-store');
$age = isset($_GET['fresh']) ? 0 : 20;
$out = [];
exec('/usr/local/bin/drobo-status json --max-age '.(int)$age.' 2>&1', $out, $rc);
if ($rc !== 0) {
  echo json_encode(['drobos' => [], 'error' => implode("\n", $out)]);
} else {
  echo implode("\n", $out);
}
