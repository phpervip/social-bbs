// Video Remote entry — async bootstrap so the Module Federation container is
// initialized before the standalone App (or the host's lazy imports) mount.
import('./bootstrap');