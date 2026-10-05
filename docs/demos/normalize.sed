s#[^[:space:]]*mythhelm-demo-(repo|state)-[0-9]+#<TMPDIR>/mythhelm-demo-\1-XXX#g
s#run_[0-9A-Z]{26}#run_XXX#g
s#att_[0-9A-Z]{26}#att_XXX#g
s#(worker pid )[0-9]+(\.[0-9]+e[+][0-9]+)?#\1NNN#g
s#(native pid )[0-9]+(\.[0-9]+e[+][0-9]+)?#\1NNN#g
s#e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855#<EMPTY_SHA256>#g
s#[0-9a-f]{40}#<SHA40>#g
s#<EMPTY_SHA256>#e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855#g
s#evidence/[0-9a-f]{12}/#evidence/<SHA12>/#g
