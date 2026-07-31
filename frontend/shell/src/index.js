/**
 * MF host entry — must stay synchronous and tiny:
 * dynamic import of bootstrap creates the required async boundary.
 */
import './shared/styles.css';

import('./bootstrap');
