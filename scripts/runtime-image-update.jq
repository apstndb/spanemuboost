def validate_report:
  if (.images|type) != "object" or
     (.images|keys) != ["DefaultEmulatorImage","DefaultOmniImage","LatestLTSOmniImage"] or
     any(.images[]; (.current_image|type) != "string" or (.image|type) != "string" or
       (.current_image|length)==0 or (.image|length)==0 or
       (.update_available|type) != "boolean" or
       .update_available != (.current_image != .image))
  then error("invalid discovery report") else . end;
def lane:
  if . == "DefaultEmulatorImage" then "Emulator" elif . == "DefaultOmniImage" then "Omni Latest" else "Omni Latest LTS" end;
def pr_body($marker;$versions):
  $marker + "\n" + $versions + "\n\nUpdate pinned runtime image references and regenerate public constants offline.\n\n" +
  "| Runtime | Current | Candidate |\n|---|---|---|\n" +
  ([.images|to_entries[]|select(.value.update_available)|"| " + (.key|lane) + " | `" + .value.current_image + "` | `" + .value.image + "` |"]|join("\n")) +
  "\n\nRegistry availability is not runtime validation or an official announcement. Review the build, Emulator, Omni Latest and Latest LTS CI before merging. Omni integration remains experimental. This PR is managed automatically; closing it dismisses this candidate set. No automatic merge or release.\n";
