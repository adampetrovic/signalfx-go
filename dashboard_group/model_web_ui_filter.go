/*
 * Dashboard Groups API
 *
 * Dashboard groups let you collect dashboards with common characteristics in one place in the web UI, so you can view them together or in sequence. ## Dashboard group membership A dashboard can belong to only one dashboard group, but a group can contain multiple dashboards. However, user default dashboard groups can only contain the user's default dashboard. <br> When you create a new dashboard group, the system generates a new dashboard for it. Similarly, when you create a new dashboard, the system creates a new dashboard group for it. Because a dashboard can only belong to one group, you can't add existing dashboards to a new group (the dashboards already belong to a group). To add an existing dashboard to a new group, create the group and then change the dashboard group of the dashboards to the new group. <br> You can add a new dashboard to an existing group at any time. ## Cloning dashboards into different groups You can also clone existing dashboards into another group. Use this feature to test dashboards in your user dashboard group before cloning them into a production group. You can also use this feature to customize an existing dashboard, by cloning it into your user group and then modifying it. ## Deleting dashboard groups When you delete a dashboard group, the system deletes the dashboards in the group and the charts that those dashboards contain.<br> **Note:** The system doesn't do this for dashboards. When you delete a dashboard, the system orphans its charts, but it doesn't delete them. <br> You can assign a dashboard group to one or more teams in your organization. The groups then appear on the team's landing page in the web UI. ## Viewing a dashboard group To view a dashboard group you create using the API, navigate to the following URL:<br> `https://app.<REALM>.signalfx.com/#/dashboard/<GROUP_ID>` <br> For `<GROUP_ID>`, substitute the value of the dashboard group ID. In the web UI, you see the dashboard group name, and underneath it the dashboard names displayed as tabs. ## Dashboard group authorizations By default, all users can edit or delete dashboard groups. If your organization has the \"write permissions\" feature enabled, your administrator can limit editing and deletion of specific dashboard groups to individual users or teams or both. This feature helps prevent unauthorized or accidental modifications to dashboard groups. Administrators can always modify write permissions, even for dashboard groups which they don't have permission to edit. This lets administrators escalate their access for any dashboard group. When a user deletes a dashboard group, the system deletes the group's dashboards without regard to the *dashboard* permissions. Only the dashgroup group permissions are considered.
 *
 * API version: 3.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package dashboard_group

import "github.com/adampetrovic/signalfx-go/util"

// The specification for a filter that appears in the web UI. The filter compares the value of a dimension or custom property to a value specified in this filter. You can specify the following in the filter:<br>   * A default value   * A list of suggested values to display in the web UI   * A flag that controls user input; if set to `true`, users are limited     to the default and suggested values. <br> You can also force users to choose this filter in order to see data in the dashboard's charts.
type WebUiFilter struct {
	// Array of strings containing values to place at the top of the **Suggested Values** dropdown in web UI for the dashboard. If `ChartsWebUIFilter.restricted`  is `true`, the filter must use one of the values in this array; users only have a choice of selecting a value from this array.
	PreferredSuggestions util.StringOrSlice `json:"preferredSuggestions,omitempty"`
	// Name of a custom property or dimension to filter against.<br> **Note:** If the dimension or custom property doesn't exist in any of the charts for the dashboard, the system doesn't display any data in the charts.
	Property string `json:"property"`
	// Flag that controls the display of chart data. If `true`, users must use this filter in order to see data; otherwise, users can delete the filter.
	// A list of values to compare to the value of the dimension or custom property specified in `ChartsWebUIFilter.property`. If the list contains more than one value, the filter becomes a set of queries between the value of `property` and each element of `value`. The system joins these queries with an implicit OR.
	Value util.StringOrSlice `json:"value"`
}
