# set blank options variables
SMTP_USER_OPT=""
SMTP_PASS_OPT=""

# SMTP username
if [ -n "${V4_SMPT_USER}" ]; then
   SMTP_USER_OPT="-smtpuser ${V4_SMPT_USER}"
fi

# SMTP password
if [ -n "${V4_SMPT_PASS}" ]; then
   SMTP_PASS_OPT="-smtppass ${V4_SMPT_PASS}"
fi

# run application
cd bin; ./v4availability \
   -virgo ${V4_URL} \
   -ils ${ILS_SERVICE} \
   -solr ${V4_SOLR_URL} \
   -core ${V4_SOLR_CORE} \
   -hsilliad ${V4_HSL_ILLIAD_URL} \
   -jwtkey ${V4_JWT_KEY} \
   -smtphost ${V4_SMPT_HOST} \
   -smtpport ${V4_SMPT_PORT} \
   -smtpsender ${V4_SMPT_SENDER} \
   -cremail ${V4_CR_EMAIL} \
   -lawemail ${V4_LAW_CR_EMAIL} \
   ${SMTP_USER_OPT} \
   ${SMTP_PASS_OPT}

#
# end of file
#
