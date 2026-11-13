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
def issue_body($marker;$versions;$run):
  $marker + "\n" + $versions + "\n\n" +
  "New runtime image versions are available compared with the constants on the default branch.\n\n" +
  "| Runtime | Current image | Available image |\n|---|---|---|\n" +
  ([.images | to_entries[] | select(.value.update_available) |
    "| " + (.key|lane) + " | `" + .value.current_image + "` | `" + .value.image + "` |"] | join("\n")) +
  "\n\nThese are registry candidates, not a claim of runtime compatibility or an official release announcement. Validate Emulator, Omni Latest and Latest LTS before updating their pinned constants. Omni integration remains experimental.\n\n" +
  "[Discovery workflow run](" + $run + "). Run `mise run images:check` to inspect full image evidence.\n\n" +
  "This tracking issue is managed automatically. Repeated checks of the same version set do not notify again. Closing it suppresses that exact candidate set; a different set can create a new issue.\n";
