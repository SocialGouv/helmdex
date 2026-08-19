import { useEffect, useRef } from "react";

// Mutations outlive their component: in the desktop shell, switching folder
// tabs remounts the whole App while an in-flight mutation keeps running. Its
// onSuccess must not fire global side effects (wouter's navigate is a
// process-wide history.pushState) against whichever tab is active by then —
// guard them with this ref.
export function useMountedRef() {
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  return mounted;
}
