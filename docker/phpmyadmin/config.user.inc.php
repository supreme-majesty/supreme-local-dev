<?php
/**
 * Hide engine/tool catalogs. Keep in sync with pkg/services/catalog.go.
 */
$cfg['Servers'][$i]['hide_db'] = '^(?:information_schema|performance_schema|mysql|sys|phpmyadmin|test|postgres|template0|template1)$';
$cfg['NavigationTreeEnableGrouping'] = false;
